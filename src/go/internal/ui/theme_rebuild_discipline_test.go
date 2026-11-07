//go:build test

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A package-level var derived from a genColor* token is evaluated ONCE at init and goes stale after ApplyTheme;
// such derivations live in rebuildThemeAliases().
func TestNoTopLevelDerivedColorsOutsideTheme(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	var bad []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || strings.HasSuffix(f, ".gen.go") || f == "theme.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				for i, v := range vs.Values {
					if _, isFn := v.(*ast.FuncLit); isFn {
						continue // function bodies evaluate at call time, not init
					}
					refs := false
					ast.Inspect(v, func(n ast.Node) bool {
						id, ok := n.(*ast.Ident)
						if !ok {
							return true
						}
						if strings.HasPrefix(id.Name, "genColor") {
							refs = true
						}
						if strings.HasPrefix(id.Name, "col") && len(id.Name) > 3 && id.Name[3] >= 'A' && id.Name[3] <= 'Z' {
							refs = true
						}
						// A style built by a helper copies colours at init just the same.
						if strings.HasSuffix(id.Name, "Style") || strings.HasSuffix(id.Name, "StyleFromSpec") {
							refs = true
						}
						return true
					})
					if refs {
						bad = append(bad, f+": "+vs.Names[i].Name)
					}
				}
			}
		}
	}
	if len(bad) > 0 {
		t.Fatalf("package-level color derivations outside rebuildThemeAliases (move them into theme.go rebuildThemeAliases):\n  %s", strings.Join(bad, "\n  "))
	}
}
