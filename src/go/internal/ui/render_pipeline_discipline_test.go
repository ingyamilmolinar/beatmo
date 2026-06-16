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

// TestRenderPipelineDiscipline asserts that the body of (*DrumView).Draw —
// the single pixel-emission entry point for the drum pane — contains no
// draw-primitive calls. Every byte that lands on screen MUST originate
// inside a Layer or Zone registered with DrumViewTree (see layer.go,
// drumview_tree.go).
//
// Adding a draw primitive (drawRect, drawRoundedRect, drawButton,
// drawSparklineInRect, drawBottomSheetPanel, DrawSplitterHandle,
// DrawTextAt, DrawTextColorAtScale, DrawIcon, DrawImage, …) to
// (*DrumView).Draw or to (*DrumView).decayAnims (the only other public
// helper called from Draw outside the tree) will fail this test.
//
// The test parses internal/ui/*.go (excluding _test.go) with go/parser
// and walks each forbidden function's body looking for *ast.CallExpr
// nodes whose name (Ident or SelectorExpr.Sel) matches the forbidden
// set. Because layer/zone helpers and existing utility functions are
// untouched, this is purely a guard on the DrumView dispatch surface.
func TestRenderPipelineDiscipline(t *testing.T) {
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
	}
	// Functions whose bodies must remain pixel-free. (file, function-name).
	// Methods on DrumView are matched by their function name; the file
	// scope identifies the receiver.
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
			// Match the (*DrumView) receiver. Both gate functions are
			// methods on DrumView; if a free function with the same
			// name is later added we want this test to keep gating
			// the method, so require a receiver.
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

// callIdent extracts the trailing identifier of a call's function
// expression: either Ident.Name or SelectorExpr.Sel.Name. Returns ""
// for calls we don't analyse (function literals, type conversions).
func callIdent(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	}
	return ""
}

// TestRenderPipelineDisciplineLayerCoverage asserts that the full
// inventory of decorative Layers documented in drumview_tree.go is
// actually instantiated in drumview_ctor.go. Catches the "added a Z
// constant but forgot to register the layer" regression.
func TestRenderPipelineDisciplineLayerCoverage(t *testing.T) {
	want := []string{
		"newBackgroundLayer",
		"newEQPeekLayer",
		"newRackMaskLayer",
		"newTransportPulseLayer",
		"newViewSwitchLayer",
		"newRowZoomChipsLayer",
		// newNotificationsLayer removed: the floating top-right toast was
		// replaced by the in-band notification area (drawn by TimelineZone)
		// + the notif-history portal popup (notification redesign).
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
