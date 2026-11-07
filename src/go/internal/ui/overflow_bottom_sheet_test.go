//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// The file menu drops from the grid folder button and stays in the GRID pane on mobile too; a drum-pane bottom
// sheet would put a grid-pane menu into the drum view.
func TestOverflowMenuStaysInGridPaneOnMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	dv := g.drum
	dv.OpenOverflowMenu()
	defer dv.closeOverflowMenu()

	r := dv.overflowPopupRect()
	if r.Empty() {
		t.Fatal("overflow popup rect empty when open")
	}
	if gridH := g.split.GridH(g.winH); r.Max.Y > gridH {
		t.Fatalf("file menu %v spills past the grid pane (gridH=%d) into the drum-view panel on mobile", r, gridH)
	}
}

// A full-width sheet would waste the chrome on a mouse-driven desktop.
func TestOverflowBottomSheet_DropdownOnDesktop(t *testing.T) {
	setupMobileTest(t, false)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	dv := g.drum
	dv.OpenOverflowMenu()
	defer dv.closeOverflowMenu()

	r := dv.overflowPopupRect()
	if r.Empty() {
		t.Fatal("desktop popup rect empty")
	}
	if r.Min.X == dv.Bounds.Min.X && r.Max.X == dv.Bounds.Max.X {
		t.Fatalf("desktop popup is unexpectedly full-width [%d,%d] — should be a dropdown", r.Min.X, r.Max.X)
	}
}

// Never below the touch floor, even when the pane is short and items overflow.
func TestOverflowBottomSheet_HasMinTouchHeight(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	dv := g.drum
	dv.OpenOverflowMenu()
	defer dv.closeOverflowMenu()

	r := dv.overflowPopupRect()
	if r.Empty() {
		t.Fatal("popup rect empty")
	}
	if r.Dy() < TouchMinTarget() {
		t.Fatalf("bottom sheet height %d < TouchMinTarget %d", r.Dy(), TouchMinTarget())
	}
}
