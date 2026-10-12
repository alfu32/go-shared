// Package goxpls implements the GoX language-server protocol endpoint.
package goxpls

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	gxparser "github.com/alfu32/go-shared/gox/internal/parser"
	gxscanner "github.com/alfu32/go-shared/gox/internal/scanner"
	"go/token"
)

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type document struct {
	text    string
	version int
}

type server struct {
	in      *bufio.Reader
	out     io.Writer
	writeMu sync.Mutex
	docs    map[string]document
}

// Serve reads and writes Language Server Protocol messages until exit or EOF.
func Serve(in io.Reader, out io.Writer) error {
	s := &server{in: bufio.NewReader(in), out: out, docs: map[string]document{}}
	for {
		msg, err := s.readMessage()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if err := s.handle(msg); err != nil {
			return err
		}
		if msg.Method == "exit" {
			return nil
		}
	}
}

func (s *server) readMessage() (rpcMessage, error) {
	length := -1
	for {
		line, err := s.in.ReadString('\n')
		if err != nil {
			return rpcMessage{}, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if key, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(key, "Content-Length") {
			if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &length); err != nil {
				return rpcMessage{}, fmt.Errorf("goxpls: invalid Content-Length: %w", err)
			}
		}
	}
	if length < 0 {
		return rpcMessage{}, fmt.Errorf("goxpls: missing Content-Length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(s.in, body); err != nil {
		return rpcMessage{}, err
	}
	var msg rpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return rpcMessage{}, fmt.Errorf("goxpls: invalid JSON-RPC message: %w", err)
	}
	return msg, nil
}

func (s *server) send(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = s.out.Write(body)
	return err
}

func (s *server) respond(id json.RawMessage, result any) error {
	return s.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *server) notify(method string, params any) error {
	return s.send(rpcMessage{JSONRPC: "2.0", Method: method, Params: mustJSON(params)})
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func (s *server) handle(msg rpcMessage) error {
	switch msg.Method {
	case "initialize":
		return s.respond(msg.ID, map[string]any{
			"capabilities": map[string]any{
				"positionEncoding": "utf-16",
				"textDocumentSync": map[string]any{"openClose": true, "change": 1},
				"semanticTokensProvider": map[string]any{
					"legend": map[string]any{"tokenTypes": semanticTokenTypes, "tokenModifiers": []string{}},
					"full":   true,
				},
			},
			"serverInfo": map[string]string{"name": "goxpls", "version": "0.1.0"},
		})
	case "initialized", "$/cancelRequest":
		return nil
	case "shutdown":
		return s.respond(msg.ID, nil)
	case "exit":
		return nil
	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Text    string `json:"text"`
				Version int    `json:"version"`
			} `json:"textDocument"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return err
		}
		d := document{text: p.TextDocument.Text, version: p.TextDocument.Version}
		s.docs[p.TextDocument.URI] = d
		return s.publishDiagnostics(p.TextDocument.URI, d)
	case "textDocument/didChange":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return err
		}
		if len(p.ContentChanges) > 0 {
			d := document{text: p.ContentChanges[len(p.ContentChanges)-1].Text, version: p.TextDocument.Version}
			s.docs[p.TextDocument.URI] = d
			return s.publishDiagnostics(p.TextDocument.URI, d)
		}
	case "textDocument/didClose":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return err
		}
		delete(s.docs, p.TextDocument.URI)
		return s.notify("textDocument/publishDiagnostics", map[string]any{"uri": p.TextDocument.URI, "diagnostics": []any{}})
	case "textDocument/semanticTokens/full":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return err
		}
		d := s.docs[p.TextDocument.URI]
		return s.respond(msg.ID, map[string]any{"data": encodeSemanticTokens(d.text)})
	default:
		if len(msg.ID) != 0 {
			return s.send(rpcMessage{JSONRPC: "2.0", ID: msg.ID, Error: &rpcError{Code: -32601, Message: "method not supported by goxpls"}})
		}
		return nil
	}
	return nil
}

func (s *server) publishDiagnostics(uri string, d document) error {
	path := uriPath(uri)
	fset := token.NewFileSet()
	_, err := gxparser.ParseFileWithBindings(fset, path, []byte(d.text), gxparser.GoX|gxparser.BareJSX|gxparser.ParseComments|gxparser.AllErrors, path, []string{"useState", "useRef", "useContext", "useEffect"})
	diagnostics := []map[string]any{}
	if err != nil {
		list, ok := err.(gxscanner.ErrorList)
		if ok {
			for _, e := range list {
				start := offsetPosition(d.text, e.Pos.Offset)
				end := start
				end["character"]++
				diagnostics = append(diagnostics, map[string]any{
					"range":    map[string]any{"start": start, "end": end},
					"severity": 1, "source": "goxpls", "message": e.Msg,
				})
			}
		} else {
			diagnostics = append(diagnostics, map[string]any{
				"range":    map[string]any{"start": map[string]int{"line": 0, "character": 0}, "end": map[string]int{"line": 0, "character": 1}},
				"severity": 1, "source": "goxpls", "message": err.Error(),
			})
		}
	}
	return s.notify("textDocument/publishDiagnostics", map[string]any{"uri": uri, "version": d.version, "diagnostics": diagnostics})
}

func uriPath(uri string) string {
	u, err := url.Parse(uri)
	if err == nil && u.Scheme == "file" {
		p, e := url.PathUnescape(u.Path)
		if e == nil {
			return filepath.FromSlash(p)
		}
	}
	return uri
}

func offsetPosition(src string, offset int) map[string]int {
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		offset = len(src)
	}
	line, character := 0, 0
	for _, r := range src[:offset] {
		if r == '\n' {
			line++
			character = 0
		} else if r <= 0xffff {
			character++
		} else {
			character += 2
		}
	}
	return map[string]int{"line": line, "character": character}
}
