//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// After unification, EVERY node-sidebar control — dropdown LIST items, steppers,
// selectors, and the close button — uses the shared menu keycap style
// (DropdownStyle) so the node pop-up reads identically to the other menus. The
// node's own color is layered on via tintBtnAccent (keycap face tint) / drawMenuRow's accent.
func TestNodeDropdownItemsUseDropdownStyle(t *testing.T) {
	g := newTestGameForUndo(t)
	g.tryAddNode(7, 7, model.NodeTypeRegular)
	node := g.nodeAt(7, 7)
	if node == nil {
		t.Fatal("failed to add test node")
	}
	g.sidebar.Open(node)
	g.sidebar.ExpandAllSections()
	g.sidebar.logicDropdownOpen = true
	g.sidebar.grooveDropdownOpen = false
	g.updateBeatInfos()
	g.sidebar.layout()

	// Every node control (a dropdown item, a stepper, a selector, the close
	// button) must be DropdownStyle.
	for _, id := range []string{"logic:probability", "vol-", "vol+", "logic", "close"} {
		b := g.sidebar.btns[id]
		if b == nil {
			t.Fatalf("node button %q missing after layout", id)
		}
		bs, ok := b.Style.(ButtonStyle)
		if !ok {
			t.Fatalf("node button %q Style is not a ButtonStyle: %T", id, b.Style)
		}
		// Checked right after layout, before any Draw — tintBtnAccent only tints
		// the cap face during Draw, so the base fill here is still DropdownStyle's
		// (the cap-face spec that defines the keycap look).
		if bs.Fill != DropdownStyle.Fill {
			t.Errorf("node button %q: Fill = %v, want DropdownStyle fill %v", id, bs.Fill, DropdownStyle.Fill)
		}
	}
}

// drawDropdownItem must route through drawMenuRow, which paints the node-color
// active stripe for the SELECTED item (drawMenuItemBackgroundAccent → a filled
// rect of exactly nodeAccent at the row's left edge). We intercept drawRect to
// confirm a node-accent-colored fill lands inside the selected item's rect.
func TestNodeDropdownItemDrawsNodeAccentStripe(t *testing.T) {
	g := newTestGameForUndo(t)
	g.tryAddNode(7, 7, model.NodeTypeRegular)
	node := g.nodeAt(7, 7)
	if node == nil {
		t.Fatal("failed to add test node")
	}
	g.sidebar.Open(node)
	g.sidebar.ExpandAllSections()
	g.sidebar.logicDropdownOpen = true
	g.sidebar.grooveDropdownOpen = false
	g.updateBeatInfos()
	// Use a taller window so the logic dropdown items fall inside the viewport
	// (with 800×600 the grid pane is only ~300px, putting the logic items just
	// below its bottom edge and triggering the Skip below).
	g.Layout(800, 1200)
	g.sidebar.layout()

	// A fresh regular node has empty LogicKind → the selected item is "logic:".
	const selID = "logic:"
	r, ok := g.sidebar.rects[selID]
	if !ok || r.Empty() {
		t.Fatalf("selected dropdown item %q has no rect", selID)
	}
	if !g.sidebar.inViewport(r) {
		t.Skipf("selected item %q scrolled out of viewport; cannot assert stripe", selID)
	}
	want := color.RGBAModel.Convert(g.sidebar.nodeAccent()).(color.RGBA)

	// Intercept drawRect to record filled rects of the node-accent color.
	orig := drawRect
	defer func() { drawRect = orig }()
	found := false
	drawRect = func(dst *ebiten.Image, rr image.Rectangle, c color.Color, filled bool) {
		if filled && color.RGBAModel.Convert(c).(color.RGBA) == want && !rr.Intersect(r).Empty() {
			found = true
		}
		orig(dst, rr, c, filled)
	}

	dst := ebiten.NewImage(64, 256)
	g.sidebar.drawDropdownItem(dst, selID, true)

	if !found {
		t.Errorf("drawDropdownItem(%q, selected) drew no node-accent stripe (want a filled %v rect inside %v)", selID, want, r)
	}
}
