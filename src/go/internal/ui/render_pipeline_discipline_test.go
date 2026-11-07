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

// DrumView.Draw and decayAnims only dispatch: every pixel on the drum pane comes from a Layer or Zone in DrumViewTree,
// so a draw primitive called from either body fails.
func TestRenderPipelineDiscipline(t *testing.T) {
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
	}
	// (file, function) pairs whose bodies stay pixel-free; the file identifies the receiver.
	gates := []struct {
		file string
		fn   string
	}{
		{"drumview_draw.go", "Draw"}, // (*DrumView).Draw — the dispatch entry point
		{"drumview_toolbar.go", "decayAnims"},
	}

	fset := token.NewFileSet()
	for _, g := range gates {
		path, err := filepath.Abs(g.file)
		if err != nil {
			t.Fatalf("filepath.Abs(%q): %v", g.file, err)
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", g.file, err)
		}
		var fn *ast.FuncDecl
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fd.Name.Name != g.fn {
				continue
			}
			// Require the receiver, so a free function of the same name cannot shadow the method.
			if fd.Recv == nil || len(fd.Recv.List) != 1 {
				continue
			}
			fn = fd
			break
		}
		if fn == nil {
			t.Fatalf("did not find (*DrumView).%s in %s", g.fn, g.file)
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
				t.Errorf("%s:%d: %s contains forbidden draw primitive %q — every pixel must originate in a Layer or Zone, not in DrumView dispatch",
					filepath.Base(pos.Filename), pos.Line, g.fn, name)
			}
			return true
		})
	}
}

// "" for literals and conversions.
func callIdent(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	}
	return ""
}

// Every documented decorative Layer is instantiated: a Z constant with no registered layer fails.
func TestRenderPipelineDisciplineLayerCoverage(t *testing.T) {
	want := []string{
		"newBackgroundLayer",
		"newEQPeekLayer",
		"newRackMaskLayer",
		"newTransportPulseLayer",
		"newViewSwitchLayer",
		"newRowZoomChipsLayer",
		"newLayoutPillsLayer",
		"newLayoutGuidesLayer",
	}
	data, err := os.ReadFile("drumview_ctor.go")
	if err != nil {
		t.Fatalf("read drumview_ctor.go: %v", err)
	}
	src := string(data)
	for _, ctor := range want {
		if !strings.Contains(src, ctor) {
			t.Errorf("drumview_ctor.go does not reference %s — layer not registered", ctor)
		}
	}
}
