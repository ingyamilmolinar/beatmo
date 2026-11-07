//go:build test

package ui

import "testing"

// Immediately left of the gear, same size, disjoint, with the folder icon.
func TestGridFileButtonPresentBothPlatforms(t *testing.T) {
	check := func(t *testing.T, g *Game) {
		if g.gridFileBtn == nil {
			t.Fatal("gridFileBtn is nil")
		}
		if g.gridFileBtn.Icon != string(IconFolder) {
			t.Fatalf("folder icon=%q want %q", g.gridFileBtn.Icon, IconFolder)
		}
		gear := g.gridHelpButtonRect()
		file := g.gridFileButtonRect()
		if file.Empty() || gear.Empty() {
			t.Fatalf("rects empty gear=%v file=%v", gear, file)
		}
		if file.Overlaps(gear) {
			t.Fatalf("file %v overlaps gear %v", file, gear)
		}
		if file.Max.X > gear.Min.X {
			t.Fatalf("file button must sit LEFT of the gear (file=%v gear=%v)", file, gear)
		}
		if file.Dx() != gear.Dx() || file.Dy() != gear.Dy() {
			t.Fatalf("file/gear size mismatch file=%v gear=%v", file, gear)
		}
	}
	t.Run("desktop", func(t *testing.T) { check(t, newGameWithStart(t)) })
	t.Run("mobile", func(t *testing.T) { check(t, newMobileGame(t)) })
}

func TestGridFileButtonOpensFileMenu(t *testing.T) {
	g := newGameWithStart(t)
	g.openFileMenu()
	if !g.drum.IsOverflowMenuOpen() {
		t.Fatal("openFileMenu did not open the overflow (file) portal")
	}
}

// Drops from its trigger, per the DESIGN.md menu-adjacency rule.
func TestFileMenuAnchoredUnderFolderButton(t *testing.T) {
	g := newGameWithStart(t)
	g.openFileMenu()
	folder := g.gridFileButtonRect()
	if folder.Empty() {
		t.Fatal("folder button rect empty")
	}
	pop := g.drum.overflowPopupRect()
	if pop.Empty() {
		t.Fatal("file popup rect empty")
	}
	// In the button's column and inside the grid pane: below the button when it fits, above otherwise, never
	// past the split.
	if pop.Min.X > folder.Max.X || pop.Max.X < folder.Min.X {
		t.Fatalf("file popup %v not horizontally adjacent to folder button %v", pop, folder)
	}
	if gridH := g.split.GridH(g.winH); pop.Max.Y > gridH {
		t.Fatalf("file popup %v spills past the grid pane (gridH=%d) into the drum-view panel", pop, gridH)
	}
}
