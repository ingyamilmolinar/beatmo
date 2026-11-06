package clone

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCloneIsDevOnly: the clone analysis is developer tooling. Only the clone
// packages themselves and the dev commands (cmd/clone; cmd/refs for the
// session / validate -loss flow; cmd/measure-loudness for the shared crest
// rule) may import it, so it never reaches the desktop binary or the WASM
// bundle.
func TestCloneIsDevOnly(t *testing.T) {
	const pkg = "github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	allowed := map[string]bool{
		"internal/audio/clone":        true,
		"internal/audio/clone/linalg": true,
		"cmd/clone":                   true,
		"cmd/refs":                    true,
		"cmd/measure-loudness":        true,
	}
	srcRoot, err := filepath.Abs("../../..") // src/go
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	err = filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(srcRoot, path)
		if allowed[filepath.ToSlash(filepath.Dir(rel))] {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return nil
		}
		for _, imp := range f.Imports {
			if p := strings.Trim(imp.Path.Value, `"`); p == pkg || strings.HasPrefix(p, pkg+"/") {
				t.Errorf("%s imports %s — the clone analysis is dev-only tooling", rel, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
