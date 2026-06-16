//go:build test

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// TestGridRenderPipelineDiscipline asserts that the body of (*Game).drawGridPane
// — the single grid-pane dispatch entry point — contains no draw-primitive
// calls. After the GridTree split (Tasks 5-8) every byte that lands inside the
// grid pane MUST originate inside a Layer registered with GridTree; drawGridPane
// only computes per-frame setup, an optional debug-only g.drawGridBackground
// call, Tracef logging, and the gridTree dispatch (SetBounds/EnsureLayouts/Draw).
//
// Adding a draw primitive (drawRect, drawRoundedRect, drawButton,
// drawSparklineInRect, drawBottomSheetPanel, drawPanel, drawAccentStripe,
// DrawSplitterHandle, DrawTextAt, DrawTextColorAtScale, DrawIcon, DrawImage,
// fillVerticalGradient, DrawTextStyled, DrawTextAtScale, drawScrim, …) directly
// to (*Game).drawGridPane will fail this test. Method calls like
// g.drawGridBackground(...) or g.gridTree.Draw(...) are SelectorExpr calls whose
// trailing identifier ("drawGridBackground", "Draw") is NOT in the forbidden
// set, so they remain allowed.
//
// The test parses game_draw_grid_pane.go with go/parser and walks the
// drawGridPane FuncDecl body for *ast.CallExpr nodes whose name (Ident or
// SelectorExpr.Sel) matches the forbidden set. callIdent is shared with
// render_pipeline_discipline_test.go.
func TestGridRenderPipelineDiscipline(t *testing.T) {
	forbidden := map[string]bool{
		"drawRect":             true,
		"drawRoundedRect":      true,
		"drawButton":           true,
		"drawSparklineInRect":  true,
		"drawBottomSheetPanel": true,
		"drawPanel":            true,
		"drawAccentStripe":     true,
		"DrawSplitterHandle":   true,
		"DrawTextAt":           true,
		"DrawTextColorAtScale": true,
		"DrawIcon":             true,
		"DrawImage":            true,
		// Grid-pane-specific primitives that the drawGrid* layers own.
		"fillVerticalGradient": true,
		"DrawTextStyled":       true,
		"DrawTextAtScale":      true,
		"drawScrim":            true,
	}

	const (
		file = "game_draw_grid_pane.go"
		fnNm = "drawGridPane"
	)

	fset := token.NewFileSet()
	path, err := filepath.Abs(file)
	if err != nil {
		t.Fatalf("filepath.Abs(%q): %v", file, err)
	}
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Name.Name != fnNm {
			continue
		}
		// Match the (*Game) receiver. drawGridPane is a method on Game; require
		// a receiver so a future free function of the same name can't shadow it.
		if fd.Recv == nil || len(fd.Recv.List) != 1 {
			continue
		}
		fn = fd
		break
	}
	if fn == nil {
		t.Fatalf("did not find (*Game).%s in %s", fnNm, file)
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := callIdent(call.Fun)
		if name == "" {
			return true
		}
		if forbidden[name] {
			pos := fset.Position(call.Pos())
			t.Errorf("%s:%d: %s contains forbidden draw primitive %q — every grid-pane pixel must originate in a GridTree Layer, not in drawGridPane dispatch",
				filepath.Base(pos.Filename), pos.Line, fnNm, name)
		}
		return true
	})
}
