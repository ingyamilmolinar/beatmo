//go:build test

package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestGridTreeDrawHasSingleDispatchSite pins the hidden coupling created by the
// dispatch-only design: the drawGrid* Layer methods (grid_pane_draw.go) read
// per-frame state from g.gridDrawCtx + g.gridDrawScreen, and ONLY
// (*Game).drawGridPane populates those before it dispatches the tree. A second
// caller of g.gridTree.Draw that skipped that setup would render every grid
// layer against a stale/zero context (wrong camera matrix, nil target screen,
// empty cull rect). This guard makes that contract structural: g.gridTree.Draw
// must be invoked from exactly one place, (*Game).drawGridPane.
//
// It also asserts drawGridPane assigns g.gridDrawScreen — so the single call
// site is provably the one that wires the shared context, not just any single
// caller.
func TestGridTreeDrawHasSingleDispatchSite(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	var sites []string
	dispatcherAssignsScreen := false
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				// Match X.gridTree.Draw(...) — Fun is SelectorExpr{Sel:"Draw"}
				// whose X is itself a SelectorExpr{Sel:"gridTree"}.
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Draw" {
					return true
				}
				inner, ok := sel.X.(*ast.SelectorExpr)
				if !ok || inner.Sel.Name != "gridTree" {
					return true
				}
				sites = append(sites, fmt.Sprintf("%s:%s", file, fd.Name.Name))
				return true
			})
			if file == "game_draw_grid_pane.go" && fd.Name.Name == "drawGridPane" {
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					as, ok := n.(*ast.AssignStmt)
					if !ok {
						return true
					}
					for _, lhs := range as.Lhs {
						if s, ok := lhs.(*ast.SelectorExpr); ok && s.Sel.Name == "gridDrawScreen" {
							dispatcherAssignsScreen = true
						}
					}
					return true
				})
			}
		}
	}

	if len(sites) != 1 || sites[0] != "game_draw_grid_pane.go:drawGridPane" {
		t.Fatalf("g.gridTree.Draw must be called exactly once, from (*Game).drawGridPane "+
			"(the only populator of g.gridDrawCtx/g.gridDrawScreen the drawGrid* layers read); got %v", sites)
	}
	if !dispatcherAssignsScreen {
		t.Fatalf("(*Game).drawGridPane must assign g.gridDrawScreen before dispatching the tree; " +
			"the single Draw call site is meaningless if it no longer wires the shared draw context")
	}
}
