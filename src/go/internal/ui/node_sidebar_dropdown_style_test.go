//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Every sidebar control uses DropdownStyle, so the pop-up reads like the other menus; the node colour is layered
// on at draw.
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

	for _, id := range []sbID{sbLogicOption(model.LogicProbability), sbMinus(fieldVol), sbPlus(fieldVol), sbLogicSelect, sbClose} {
		b := g.sidebar.btns[id]
		if b == nil {
			t.Fatalf("node button %q missing after layout", id)
		}
		bs, ok := b.Style.(ButtonStyle)
		if !ok {
			t.Fatalf("node button %q Style is not a ButtonStyle: %T", id, b.Style)
		}
		// Before any Draw, so the base fill is still DropdownStyle's.
		if bs.Fill != DropdownStyle.Fill {
			t.Errorf("node button %q: Fill = %v, want DropdownStyle fill %v", id, bs.Fill, DropdownStyle.Fill)
		}
	}
}

// Through drawMenuRow, which paints the selected item's left stripe in exactly nodeAccent.
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
	// At 800x600 the grid pane is ~300 px and the logic items fall below it.
	g.Layout(800, 1200)
	g.sidebar.layout()

	// A fresh regular node has empty LogicKind → the selected item is "logic:".
	selID := sbLogicOption(model.LogicNone)
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
	g.sidebar.drawDropdownItem(dst, selID, true, g.sidebar.nodeAccent())

	if !found {
		t.Errorf("drawDropdownItem(%q, selected) drew no node-accent stripe (want a filled %v rect inside %v)", selID, want, r)
	}
}
