//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Icons, not hand-drawn glyph text.
func TestSidebarIncDecButtons(t *testing.T) {
	g := newTestGameForUndo(t)
	g.tryAddNode(7, 7, model.NodeTypeRegular)
	node := g.nodeAt(7, 7)
	if node == nil {
		t.Fatal("failed to add test node")
	}
	g.sidebar.Open(node)
	g.sidebar.ExpandAllSections()
	g.updateBeatInfos()
	// Run layout to wire the button rects and closures.
	g.sidebar.layout()

	for _, key := range []sbID{sbMinus(fieldVol), sbPlus(fieldVol), sbMinus(fieldPit), sbPlus(fieldPit), sbMinus(fieldDur), sbPlus(fieldDur)} {
		b := g.sidebar.btns[key]
		if b == nil {
			t.Fatalf("sidebar button %q missing after layout", key)
		}
		wantIcon := "plus"
		if key.el == elMinus {
			wantIcon = "minus"
		}
		if b.Icon != wantIcon {
			t.Errorf("button %q: Icon = %q, want %q", key, b.Icon, wantIcon)
		}
		if b.Text != "" {
			t.Errorf("button %q: Text = %q, want empty (icon-only)", key, b.Text)
		}
	}
}
