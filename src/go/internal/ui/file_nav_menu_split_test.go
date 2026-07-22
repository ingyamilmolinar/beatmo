//go:build test

package ui

import "testing"

// TestNavMenuIsSeparatePortal asserts the navigation menu (opened by tapping the
// beat counter) is its own portal component and does NOT open the overflow
// (file) menu.
func TestNavMenuIsSeparatePortal(t *testing.T) {
	g := newGameWithStart(t)
	g.drum.OpenNavigation()
	if !g.drum.IsNavMenuOpen() {
		t.Fatal("nav menu must open its own portal component")
	}
	if g.drum.IsOverflowMenuOpen() {
		t.Fatal("nav menu must NOT open the overflow (file) menu")
	}
}

// TestNavMenuAnchoredUnderBeatCounter asserts the nav menu card drops from the
// beat counter (its trigger), per the DESIGN.md menu-adjacency rule.
func TestNavMenuAnchoredUnderBeatCounter(t *testing.T) {
	g := newGameWithStart(t)
	g.drum.OpenNavigation()
	bc := g.drum.beatCounterRect
	if bc.Empty() {
		t.Skip("beat counter rect not laid out in this harness")
	}
	card := g.drum.navMenuComp.Bounds()
	if card.Empty() {
		t.Fatal("nav menu card rect empty")
	}
	// Horizontal spans must overlap: the card sits in the counter's column.
	if card.Min.X > bc.Max.X || card.Max.X < bc.Min.X {
		t.Fatalf("nav menu card %v not horizontally adjacent to beat counter %v", card, bc)
	}
}

// TestFileMenuHasFileRows asserts the file menu (opened by the grid folder
// button) lists the file actions.
func TestFileMenuHasFileRows(t *testing.T) {
	g := newGameWithStart(t)
	g.drum.OpenFileMenu()
	if !g.drum.IsOverflowMenuOpen() {
		t.Fatal("file menu must open the overflow portal")
	}
	var rows int
	for _, it := range g.drum.overflowItems() {
		if it.header || it.search {
			continue
		}
		rows++
	}
	if rows == 0 {
		t.Fatal("file menu must list the file actions (Upload/Import/Export/Load Template)")
	}
}
