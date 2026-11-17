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

// TestNodeTypeRegularIsNeverComparedDirectly keeps the chord node a
// first-class firing node (docs/superpowers/specs/2026-10-09-chord-nodes-design.md).
//
// A chord fires exactly like a regular node; only where a note becomes sound
// does its voice stack matter. Every "does this node sound / trigger / show
// as on?" check must therefore ask NodeType.Fires(), never compare against
// NodeTypeRegular — a single missed `== NodeTypeRegular` silently makes a
// chord node not highlight, not schedule, or drop out of parity. A switch
// clause listing NodeTypeRegular must list NodeTypeChord too.
//
// Sites that genuinely mean "exactly a plain regular node" are listed in
// regularOnlySites with the reason.
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
