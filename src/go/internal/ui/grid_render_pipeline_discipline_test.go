//go:build test

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// drawGridPane only sets up and dispatches; every pixel in the grid pane comes from a GridTree layer. A
// selector call whose trailing name is not a draw primitive (g.gridTree.Draw) stays allowed.
func TestGridRenderPipelineDiscipline(t *testing.T) {
	forbidden := map[string]bool{
		"drawRect":             true,
		"drawRoundedRect":      true,
		"drawButton":           true,
		"drawSparklineInRect":  true,
		"drawBottomSheetPanel": true,
		"drawPanel":            true,
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
		// Require the receiver, so a free function of the same name cannot shadow it.
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
