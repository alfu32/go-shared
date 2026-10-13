package gox

import (
	"go/ast"
	stdparser "go/parser"
	"go/token"
	"path/filepath"

	"github.com/alfu32/go-shared/gox/internal/parser"
	"github.com/alfu32/go-shared/gox/internal/scanner"
)

type SyntaxDiagnostic struct {
	Offset  int
	Message string
}

// ParseGoXFile parses Go source with GoX extensions lowered into their Go AST
// representation. The standard parser mode flags control comments, errors,
// and object resolution; GoX and bare JSX parsing are always enabled.
func ParseGoXFile(fset *token.FileSet, filename string, src []byte, mode stdparser.Mode) (*ast.File, error) {
	return parser.ParseFileWithBindings(fset, filename, src,
		parser.GoX|parser.BareJSX|parser.Mode(mode), filepath.ToSlash(filename),
		[]string{"useState", "useRef", "useContext", "useEffect"})
}

// SyntaxDiagnostics parses GoX without lowering it into generated Go source.
func SyntaxDiagnostics(filename string, src []byte) []SyntaxDiagnostic {
	fset := token.NewFileSet()
	identity := filepath.ToSlash(filename)
	_, err := parser.ParseFileWithBindings(fset, filename, src,
		parser.GoX|parser.BareJSX|parser.ParseComments|parser.AllErrors,
		identity, []string{"useState", "useRef", "useContext", "useEffect"})
	list, ok := err.(scanner.ErrorList)
	if !ok {
		return nil
	}
	diagnostics := make([]SyntaxDiagnostic, 0, len(list))
	for _, e := range list {
		diagnostics = append(diagnostics, SyntaxDiagnostic{Offset: e.Pos.Offset, Message: e.Msg})
	}
	return diagnostics
}
