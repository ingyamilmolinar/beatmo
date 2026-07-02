//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestSidebarIncDecButtons verifies that after the sidebar is opened with a
// node selected and all sections are expanded, the +/− stepper buttons use
// icon-based drawing ("plus"/"minus") instead of hand-drawn glyph text.
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

	for _, key := range []string{"vol-", "vol+", "pit-", "pit+", "dur-", "dur+"} {
		b := g.sidebar.btns[key]
		if b == nil {
			t.Fatalf("sidebar button %q missing after layout", key)
		}
		wantIcon := "plus"
		if key[len(key)-1] == '-' {
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
