package fsys

import (
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
)

// SourceExpander is installed by the GoX command adapter. The filesystem package
// has no dependency on the language front end or command configuration.
var SourceExpander func(string, []fs.DirEntry) ([]fs.DirEntry, error)
var IsGoXSource func(string, []byte) bool
var generatedSources sync.Map

type generatedSource struct{ actual, original string }

func RegisterGenerated(virtual, actual string, original ...string) {
	origin := ""
	if len(original) > 0 {
		origin = original[0]
	}
	generatedSources.Store(abs(virtual), generatedSource{actual: actual, original: origin})
}
func Generated(name string) (string, bool) {
	v, ok := generatedSources.Load(abs(name))
	if !ok {
		return "", false
	}
	return v.(generatedSource).actual, true
}
func HasGoX(dir string) bool {
	entries, e := readDir(dir)
	if e != nil {
		return false
	}
	for _, d := range entries {
		if d.IsDir() {
			continue
		}
		if strings.HasSuffix(d.Name(), ".gox") {
			return true
		}
		if IsGoXSource != nil && strings.HasSuffix(d.Name(), ".go") {
			if src, err := ReadFile(filepath.Join(dir, d.Name())); err == nil && IsGoXSource(d.Name(), src) {
				return true
			}
		}
	}
	return false
}
func GoXOriginal(name string) string {
	if value, ok := generatedSources.Load(abs(name)); ok {
		if original := value.(generatedSource).original; original != "" {
			return original
		}
		return strings.TrimSuffix(name, filepath.Ext(name)) + ".gox"
	}
	return ""
}
