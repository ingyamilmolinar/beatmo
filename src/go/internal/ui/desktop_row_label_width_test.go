//go:build test

package ui

import (
	"testing"
)

// TestDesktopRowLabelFitsLongName asserts the desktop instrument-name label
// renders in full (no ellipsis clip) for a realistically long name.
//
// Regression guard for widening the left column (desktopProfile ColWeights
// {1,3} -> {2,3} in layout_profile.go). The label flexes to fill col0 minus the
// right-anchored control cluster (see layoutRowControls); at the old 25% column
// it was squeezed to ~110px and clipped all but the shortest names. The 40%
// column gives roughly triple the label area so full instrument names render.
func TestDesktopRowLabelFitsLongName(t *testing.T) {
	forceSmallScreenForTest = false
	activeProfile = nil
	defer func() { activeProfile = nil }()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	// 20 characters — comfortably longer than any built-in name and well wider
	// than the pre-change ~110px label could show, but inside the widened one.
	const longName = "Sidechain Bass Pluck"
	if len(g.drum.Rows) == 0 {
		t.Fatal("expected at least one demo row")
	}
	g.drum.Rows[0].Name = longName

	// Re-run layout so the rack picks up the new name and the col0 width feeds
	// through to the per-row label rects.
	g.drum.calcLayout()
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}

	groups := g.drum.rowGroups()
	if len(groups) == 0 || groups[0].Label == nil || groups[0].Label.Rect().Empty() {
		t.Fatal("expected a non-empty row label rect for the first (visible) row")
	}
	labelRect := groups[0].Label.Rect()

	// The label Button clips text to Dx()-2*SpaceXS (Button.Draw ->
	// clipTextToWidth). Full render requires the name to fit within that.
	avail := labelRect.Dx() - 2*SpaceXS
	need := TextWidth(longName)
	if need > avail {
		t.Errorf("desktop label too narrow for %q: need %dpx, have %dpx (label rect Dx=%d)",
			longName, need, avail, labelRect.Dx())
	}
}
