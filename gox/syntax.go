package gox

import (
	"go/token"
	"path/filepath"

	"github.com/alfu32/go-shared/gox/internal/parser"
	"github.com/alfu32/go-shared/gox/internal/scanner"
)

type SyntaxDiagnostic struct {
	Offset  int
	Message string
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
