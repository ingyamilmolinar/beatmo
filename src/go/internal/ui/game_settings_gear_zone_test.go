//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// The settings gear in the grid pane's top-right corner is a protected region:
// no grid-pane popup (node-group menu, node long-press popup, group badge) may
// invade its rectangle — neither painting on top of it (z-order) nor sliding
// its panel underneath it (spatial overlap). These tests pin both halves.

// TestAvoidReservedRect exercises the pure spatial-exclusion helper directly.
func TestAvoidReservedRect(t *testing.T) {
	const gridW, gridH = 400, 300
	// A top-right-corner reserved zone (like the inflated gear rect).
	reserved := image.Rect(360, 4, 400, 44)

	// 1. No overlap → returned unchanged.
	clear := image.Rect(0, 100, 100, 150)
	if got := avoidReservedRect(clear, reserved, gridW, gridH); got != clear {
		t.Fatalf("no-overlap: got %v, want unchanged %v", got, clear)
	}

	// 2. Empty reserved → returned unchanged (nothing to avoid).
	overlapping := image.Rect(340, 0, 400, 40)
	if got := avoidReservedRect(overlapping, image.Rectangle{}, gridW, gridH); got != overlapping {
		t.Fatalf("empty-reserved: got %v, want unchanged %v", got, overlapping)
	}

	// 3. Short panel that overlaps → pushed DOWN below the reserved zone, still
	//    inside the pane, no longer overlapping.
	short := image.Rect(300, 0, 400, 40)
	gotShort := avoidReservedRect(short, reserved, gridW, gridH)
	if gotShort.Overlaps(reserved) {
		t.Fatalf("push-down: %v still overlaps reserved %v", gotShort, reserved)
	}
	if gotShort.Min.Y < reserved.Max.Y {
		t.Fatalf("push-down: expected top >= %d, got %v", reserved.Max.Y, gotShort)
	}
	if !gotShort.In(image.Rect(0, 0, gridW, gridH)) {
		t.Fatalf("push-down: %v left the pane [0,0,%d,%d]", gotShort, gridW, gridH)
	}

	// 4. Full-height panel that overlaps → cannot push down (won't fit), so it
	//    is pushed LEFT clear of the reserved zone, still inside the pane.
	tall := image.Rect(300, 0, 400, 300)
	gotTall := avoidReservedRect(tall, reserved, gridW, gridH)
	if gotTall.Overlaps(reserved) {
		t.Fatalf("push-left: %v still overlaps reserved %v", gotTall, reserved)
	}
	if gotTall.Max.X > reserved.Min.X {
		t.Fatalf("push-left: expected right <= %d, got %v", reserved.Min.X, gotTall)
	}
	if !gotTall.In(image.Rect(0, 0, gridW, gridH)) {
		t.Fatalf("push-left: %v left the pane [0,0,%d,%d]", gotTall, gridW, gridH)
	}
}

// TestGridHelpReservedRectCoversGear: the reserved rect fully contains the gear
// button rect (it is the gear inflated by a small margin).
func TestGridHelpReservedRectCoversGear(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	gear := g.gridHelpButtonRect()
	reserved := g.gridHelpReservedRect()
	if gear.Empty() {
		t.Fatal("gear rect unexpectedly empty")
	}
	if !gear.In(reserved) {
		t.Fatalf("reserved rect %v does not contain gear rect %v", reserved, gear)
	}
}

// TestGroupMenuAvoidsSettingsGear: a group menu anchored right at the gear
// corner must not overlap the gear's rect.
func TestGroupMenuAvoidsSettingsGear(t *testing.T) {
	g, aID, bID, cID := buildGroupLoopCircuitSized(t, 1280, 720)
	gid, err := g.graph.CreateGroup("grp", []model.NodeID{aID, bID, cID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	gear := g.gridHelpButtonRect()
	// Anchor the menu at the gear corner so, absent avoidance, it clamps into
	// the gear's rect.
	g.groupMenu.OpenAt(gid, gear.Min.X, gear.Min.Y)
	advanceFrames(g, 1)

	if g.groupMenu.panel.Overlaps(gear) {
		t.Fatalf("group menu panel %v overlaps settings gear %v", g.groupMenu.panel, gear)
	}
}

// TestLongPressPopupAvoidsSettingsGear: the node long-press popup, triggered on
// a node positioned under the gear, must not overlap the gear's rect.
func TestLongPressPopupAvoidsSettingsGear(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.updateBeatInfos()

	gear := g.gridHelpButtonRect()
	// Place node (0,0) directly at the gear so the popup clamps into it.
	g.cam.OffsetX = float64(gear.Min.X + gear.Dx()/2)
	g.cam.OffsetY = float64(gear.Min.Y + gear.Dy()/2 - gridTopOffset())

	g.showLongPressPopup(n, 0, 0)

	if !g.longPressPopup {
		t.Fatal("long-press popup did not open")
	}
	if g.longPressPopupRect.Overlaps(gear) {
		t.Fatalf("long-press popup %v overlaps settings gear %v", g.longPressPopupRect, gear)
	}
}

// TestGroupBadgesAvoidSettingsGear: a group whose members sit under the gear
// produces a floating badge; sweeping the camera across the corner, no badge
// rect may overlap the gear at any offset.
func TestGroupBadgesAvoidSettingsGear(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	// Vertically-stacked members so the group's bounding-box top-right corner
	// (where the badge anchors) tracks a single column near the gear.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(0, 1, model.NodeTypeRegular)
	c := g.tryAddNode(0, 2, model.NodeTypeRegular)
	g.updateBeatInfos()
	if _, err := g.graph.CreateGroup("grp", []model.NodeID{a.ID, b.ID, c.ID}); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	gear := g.gridHelpButtonRect()
	base := gear.Min
	for dx := -70; dx <= 70; dx += 10 {
		for dy := -70; dy <= 70; dy += 10 {
			g.cam.OffsetX = float64(base.X + dx)
			g.cam.OffsetY = float64(base.Y + dy - gridTopOffset())
			for _, br := range g.groupBadgeRects() {
				if br.rect.Overlaps(gear) {
					t.Fatalf("badge %v overlaps settings gear %v at cam offset (%d,%d)",
						br.rect, gear, base.X+dx, base.Y+dy)
				}
			}
		}
	}
}

// TestSettingsGearIsTopmostGridLayer pins the z-order half of the invariant:
// the gear is the strictly-highest grid layer, so no future layer can be added
// that paints on top of it.
func TestSettingsGearIsTopmostGridLayer(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	layers := g.gridTree.LayersForTest()
	if len(layers) == 0 {
		t.Fatal("no grid layers registered")
	}
	top := layers[len(layers)-1]
	if top.ID() != "grid-help-button" {
		t.Fatalf("topmost grid layer is %q (z=%d), want %q", top.ID(), top.ZIndex(), "grid-help-button")
	}
	for _, l := range layers {
		if l.ID() == "grid-help-button" {
			continue
		}
		if l.ZIndex() >= top.ZIndex() {
			t.Fatalf("layer %q z=%d is >= settings gear z=%d; nothing may draw at or above the gear",
				l.ID(), l.ZIndex(), top.ZIndex())
		}
	}
}
