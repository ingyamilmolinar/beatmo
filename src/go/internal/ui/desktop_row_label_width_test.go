//go:build test

package ui

import (
	"testing"
)

// A long name renders in full: the label fills col0 minus the right-anchored controls, and the wider column gives
// it room.
func TestDesktopRowLabelFitsLongName(t *testing.T) {
	forceSmallScreenForTest = false
	activeProfile = nil
	defer func() { activeProfile = nil }()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	// Longer than any built-in name.
	const longName = "Sidechain Bass Pluck"
	if len(g.drum.Rows) == 0 {
		t.Fatal("expected at least one demo row")
	}
	g.drum.Rows[0].Name = longName

	g.drum.calcLayout()
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}

	groups := g.drum.rowGroups()
	if len(groups) == 0 || groups[0].Label == nil || groups[0].Label.Rect().Empty() {
		t.Fatal("expected a non-empty row label rect for the first (visible) row")
	}
	labelRect := groups[0].Label.Rect()

	// Button.Draw clips to Dx() − 2·SpaceXS.
	avail := labelRect.Dx() - 2*SpaceXS
	need := TextWidth(longName)
	if need > avail {
		t.Errorf("desktop label too narrow for %q: need %dpx, have %dpx (label rect Dx=%d)",
			longName, need, avail, labelRect.Dx())
	}
}
