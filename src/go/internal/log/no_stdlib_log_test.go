package log

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The standard "log" writes straight to stderr, ignoring -log and bypassing the test silencing and failure tail. Use
// this package. Standalone dev tools under cmd/ (not the app) may still use it.
func TestNoStdlibLogInProduction(t *testing.T) {
	root := filepath.Join("..", "..") // src/go
	var dirs []string
	for _, d := range []string{"internal", "core"} {
		dirs = append(dirs, filepath.Join(root, d))
	}
	appFiles, _ := filepath.Glob(filepath.Join(root, "cmd", "*.go"))
	check := func(path string) {
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			return
		}
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == "log" {
				t.Errorf(`%s imports the standard library "log" — log through internal/log instead`, path)
			}
		}
	}
	for _, p := range appFiles {
		check(p)
	}
	for _, d := range dirs {
		err := filepath.Walk(d, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if path == filepath.Join(root, "internal", "log") {
					return filepath.SkipDir // the one sanctioned user (log.go wraps it)
				}
				return nil
			}
			check(path)
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}
