//go:build test

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A chord fires like a regular node, so every sounds/triggers/shows-on check asks NodeType.Fires(): one
// `== NodeTypeRegular` drops chords from highlight, scheduling or parity. A clause listing Regular lists Chord
// too; genuinely regular-only sites are listed with a reason.
func TestNodeTypeRegularIsNeverComparedDirectly(t *testing.T) {
	roots := []string{"../../core", ".."}
	fset := token.NewFileSet()
	var bad []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			base := filepath.Base(path)
			if base == "scene_catalog.go" || base == "chord.go" || base == "node_types.go" {
				return nil
			}
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				t.Fatalf("parse %s: %v", path, perr)
			}
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				site := base + ":" + fd.Name.Name
				if regularOnlySites[site] != "" {
					continue
				}
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					switch x := n.(type) {
					case *ast.BinaryExpr:
						if (x.Op == token.EQL || x.Op == token.NEQ) && (isNodeTypeIdent(x.X, "NodeTypeRegular") || isNodeTypeIdent(x.Y, "NodeTypeRegular")) {
							bad = append(bad, fset.Position(x.Pos()).String()+" ("+site+"): use NodeType.Fires()")
						}
					case *ast.CaseClause:
						hasReg, hasChord := false, false
						for _, e := range x.List {
							hasReg = hasReg || isNodeTypeIdent(e, "NodeTypeRegular")
							hasChord = hasChord || isNodeTypeIdent(e, "NodeTypeChord")
						}
						if hasReg && !hasChord {
							bad = append(bad, fset.Position(x.Pos()).String()+" ("+site+"): case lists NodeTypeRegular without NodeTypeChord")
						}
					}
					return true
				})
			}
			return nil
		})
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Error(b)
	}
}

func isNodeTypeIdent(e ast.Expr, name string) bool {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name == name
	case *ast.SelectorExpr:
		return v.Sel.Name == name
	}
	return false
}

// regularOnlySites: file:func → why it means exactly a plain regular node.
var regularOnlySites = map[string]string{
	"node_type_cycle.go:nextNodeType": "the type cycle's successor table: each case is one exact state",
}
