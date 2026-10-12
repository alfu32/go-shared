package goxpls

import (
	"go/token"
	"sort"
	"strings"
	"unicode/utf8"
)

var semanticTokenTypes = []string{
	"namespace", "type", "variable", "property", "function", "keyword", "macro",
	"string", "number", "operator", "comment", "tag", "attribute",
}

var semanticTokenIndex = func() map[string]int {
	m := make(map[string]int, len(semanticTokenTypes))
	for i, name := range semanticTokenTypes {
		m[name] = i
	}
	return m
}()

type semanticToken struct {
	start, end int
	typ        int
}

type highlighter struct {
	src        string
	tokens     []semanticToken
	extensions []extensionSpan
}

type extensionSpan struct {
	start, end int
	kind       byte
}

// HighlightToken is a source range and semantic category for GoX syntax.
type HighlightToken struct {
	Start, End int
	Kind       string
}

// GoXHighlightTokens returns tokens inside F and X expressions, including
// their embedded Go expressions. Ordinary Go tokens are left to gopls.
func GoXHighlightTokens(src []byte) []HighlightToken {
	h := &highlighter{src: string(src)}
	h.scanCode(0, len(src))
	var result []HighlightToken
	for _, t := range h.tokens {
		for _, ext := range h.extensions {
			if t.start >= ext.start && t.end <= ext.end {
				kind := semanticTokenTypes[t.typ]
				result = append(result, HighlightToken{Start: t.start, End: t.end, Kind: kind})
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Start < result[j].Start })
	return result
}

// GoXLiteralRanges returns byte ranges for F and X expressions.
func GoXLiteralRanges(src []byte) [][3]int {
	h := &highlighter{src: string(src)}
	h.scanCode(0, len(src))
	ranges := make([][3]int, 0, len(h.extensions))
	for _, ext := range h.extensions {
		ranges = append(ranges, [3]int{ext.start, ext.end, int(ext.kind)})
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i][0] < ranges[j][0] })
	return ranges
}

// MaskGoXLiterals replaces each GoX expression with a same-length Go
// expression, preserving line breaks and all source offsets for gopls.
func MaskGoXLiterals(src []byte) []byte {
	out := append([]byte(nil), src...)
	last := -1
	for _, r := range GoXLiteralRanges(src) {
		start, end, kind := r[0], r[1], byte(r[2])
		if start < last || start >= end { continue }
		placeholder := `""`
		if kind == 'X' { placeholder = "nil" }
		if end-start < len(placeholder) { continue }
		for i := start; i < end; i++ {
			if out[i] != '\n' && out[i] != '\r' { out[i] = ' ' }
		}
		copy(out[start:], placeholder)
		last = end
	}
	return out
}

func encodeSemanticTokens(src string) []int {
	h := &highlighter{src: src}
	h.scanCode(0, len(src))
	sort.Slice(h.tokens, func(i, j int) bool { return h.tokens[i].start < h.tokens[j].start })
	data := make([]int, 0, len(h.tokens)*5)
	lastLine, lastChar, lastEnd := 0, 0, -1
	for _, t := range h.tokens {
		if t.start < lastEnd || t.end <= t.start {
			continue
		}
		segmentStart := t.start
		for i := t.start; i <= t.end; i++ {
			if i != t.end && src[i] != '\n' {
				continue
			}
			if segmentStart < i {
				start := offsetPosition(src, segmentStart)
				line, char := start["line"], start["character"]
				length := utf16Length(src[segmentStart:i])
				deltaLine, deltaChar := line-lastLine, char
				if deltaLine == 0 {
					deltaChar -= lastChar
				}
				data = append(data, deltaLine, deltaChar, length, t.typ, 0)
				lastLine, lastChar = line, char
			}
			segmentStart = i + 1
		}
		lastEnd = t.end
	}
	return data
}

func (h *highlighter) add(start, end int, typ string) {
	if start < 0 || end > len(h.src) || end <= start {
		return
	}
	h.tokens = append(h.tokens, semanticToken{start, end, semanticTokenIndex[typ]})
}

func (h *highlighter) scanCode(start, end int) {
	for i := start; i < end; {
		c := h.src[i]
		if isSpace(c) {
			i++
			continue
		}
		if c == 'F' && (i == start || !isIdentContinue(h.src[i-1])) && i+1 < end && (h.src[i+1] == '"' || h.src[i+1] == '`') {
			i = h.scanFormat(i, end)
			continue
		}
		if c == 'X' && (i == start || !isIdentContinue(h.src[i-1])) && i+1 < end && h.src[i+1] == '"' {
			i = h.scanMarkup(i, end)
			continue
		}
		if c == '/' && i+1 < end && h.src[i+1] == '/' {
			j := i + 2
			for j < end && h.src[j] != '\n' {
				j++
			}
			h.add(i, j, "comment")
			i = j
			continue
		}
		if c == '/' && i+1 < end && h.src[i+1] == '*' {
			j := strings.Index(h.src[i+2:end], "*/")
			if j < 0 {
				j = end
			} else {
				j += i + 4
			}
			h.add(i, j, "comment")
			i = j
			continue
		}
		if c == '"' || c == '\'' || c == '`' {
			j := h.scanQuoted(i, end, c)
			h.add(i, j, "string")
			i = j
			continue
		}
		if isIdentStart(c) {
			j := i + 1
			for j < end && isIdentContinue(h.src[j]) {
				j++
			}
			word := h.src[i:j]
			typ := "variable"
			if word == "vdom" {
				typ = "namespace"
			} else if token.Lookup(word).IsKeyword() {
				typ = "keyword"
			} else if k := skipSpace(h.src, j, end); k < end && h.src[k] == '(' {
				typ = "function"
			}
			h.add(i, j, typ)
			i = j
			continue
		}
		if c >= '0' && c <= '9' {
			j := i + 1
			for j < end && (isIdentContinue(h.src[j]) || h.src[j] == '.') {
				j++
			}
			h.add(i, j, "number")
			i = j
			continue
		}
		if strings.ContainsRune("+-*/%&|^<>=!~", rune(c)) {
			h.add(i, i+1, "operator")
		}
		i++
	}
}

func (h *highlighter) scanFormat(start, limit int) int {
	h.add(start, start+1, "macro")
	q := h.src[start+1]
	textStart, i := start+1, start+2
	for i < limit {
		if h.src[i] == '\\' && q == '"' {
			i += 2
			continue
		}
		if h.src[i] == q {
			h.add(textStart, i+1, "string")
			h.extensions = append(h.extensions, extensionSpan{start, i + 1, 'F'})
			return i + 1
		}
		if h.src[i] == '{' || h.src[i] == '}' {
			if i+1 < limit && h.src[i+1] == h.src[i] {
				i += 2
				continue
			}
			if h.src[i] == '{' {
				h.add(textStart, i, "string")
				h.add(i, i+1, "operator")
				close := findHoleEnd(h.src, i+1, limit)
				if close < 0 {
					h.add(i+1, limit, "variable")
					return limit
				}
				h.scanCode(i+1, close)
				h.add(close, close+1, "operator")
				i, textStart = close+1, close+1
				continue
			}
		}
		i++
	}
	h.add(textStart, limit, "string")
	h.extensions = append(h.extensions, extensionSpan{start, limit, 'F'})
	return limit
}

func (h *highlighter) scanMarkup(start, limit int) int {
	h.add(start, start+1, "macro")
	i, depth, rootClosed := start+2, 0, false
	for i < limit {
		if rootClosed && h.src[i] == '"' {
			h.extensions = append(h.extensions, extensionSpan{start, i + 1, 'X'})
			return i + 1
		}
		if h.src[i] == '{' {
			h.add(i, i+1, "operator")
			close := findHoleEnd(h.src, i+1, limit)
			if close < 0 {
				return limit
			}
			h.scanCode(i+1, close)
			h.add(close, close+1, "operator")
			i = close + 1
			continue
		}
		if h.src[i] != '<' {
			j := i + 1
			for j < limit && h.src[j] != '<' && h.src[j] != '{' && !(rootClosed && h.src[j] == '"') {
				j++
			}
			h.add(i, j, "string")
			i = j
			continue
		}
		closing := i+1 < limit && h.src[i+1] == '/'
		h.add(i, i+1, "operator")
		i++
		if closing {
			h.add(i, i+1, "operator")
			i++
		}
		tagStart := i
		for i < limit && isMarkupName(h.src[i]) {
			i++
		}
		h.add(tagStart, i, "tag")
		if !closing && depth == 0 {
			rootClosed = false
		}
		selfClose := false
		for i < limit {
			i = skipSpace(h.src, i, limit)
			if i >= limit {
				return limit
			}
			if h.src[i] == '>' {
				h.add(i, i+1, "operator")
				i++
				if closing {
					if depth > 0 {
						depth--
					}
					if depth == 0 {
						rootClosed = true
					}
				} else if !selfClose {
					depth++
				} else if depth == 0 {
					rootClosed = true
				}
				break
			}
			if h.src[i] == '/' && i+1 < limit && h.src[i+1] == '>' {
				h.add(i, i+1, "operator")
				h.add(i+1, i+2, "operator")
				i += 2
				selfClose = true
				if depth == 0 {
					rootClosed = true
				}
				break
			}
			if h.src[i] == '{' {
				h.add(i, i+1, "operator")
				close := findHoleEnd(h.src, i+1, limit)
				if close < 0 {
					return limit
				}
				h.scanCode(i+1, close)
				h.add(close, close+1, "operator")
				i = close + 1
				continue
			}
			if h.src[i] == '=' {
				h.add(i, i+1, "operator")
				i++
				continue
			}
			if h.src[i] == '"' || h.src[i] == '`' {
				q := h.src[i]
				j := h.scanQuoted(i, limit, q)
				h.add(i, j, "string")
				i = j
				continue
			}
			if isMarkupName(h.src[i]) {
				j := i + 1
				for j < limit && isMarkupName(h.src[j]) {
					j++
				}
				h.add(i, j, "attribute")
				i = j
				continue
			}
			h.add(i, i+1, "operator")
			i++
		}
	}
	h.extensions = append(h.extensions, extensionSpan{start, limit, 'X'})
	return limit
}

func (h *highlighter) scanQuoted(start, limit int, quote byte) int {
	i := start + 1
	for i < limit {
		if quote != '`' && h.src[i] == '\\' {
			i += 2
			continue
		}
		if h.src[i] == quote {
			return i + 1
		}
		i++
	}
	return limit
}

func findHoleEnd(src string, start, limit int) int {
	depth := 1
	for i := start; i < limit; {
		c := src[i]
		if c == '"' || c == '\'' || c == '`' {
			i++
			for i < limit {
				if c != '`' && src[i] == '\\' {
					i += 2
					continue
				}
				if src[i] == c {
					i++
					break
				}
				i++
			}
			continue
		}
		if c == '/' && i+1 < limit && src[i+1] == '/' {
			i += 2
			for i < limit && src[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < limit && src[i+1] == '*' {
			j := strings.Index(src[i+2:limit], "*/")
			if j < 0 {
				return -1
			}
			i += j + 4
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
		i++
	}
	return -1
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }
func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= utf8.RuneSelf
}
func isIdentContinue(c byte) bool { return isIdentStart(c) || c >= '0' && c <= '9' }
func isMarkupName(c byte) bool    { return isIdentContinue(c) || strings.ContainsRune("-.:", rune(c)) }
func skipSpace(s string, i, end int) int {
	for i < end && isSpace(s[i]) {
		i++
	}
	return i
}
func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xffff {
			n += 2
		} else {
			n++
		}
	}
	return n
}
func lineText(src string, line int) string {
	lines := strings.Split(src, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	return lines[line]
}
