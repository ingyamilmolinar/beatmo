//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestOverflowBottomSheet_AnchoredToBottomOnMobile verifies that on mobile
// the overflow popup is rendered as a full-width bottom sheet anchored to
// the drum-pane bottom (B6 in the screenshot critique). DESIGN.md
// §"Mobile bottom sheets" mandates `rounded.xl` full-width modals for
// this kind of menu rather than a small floating dropdown.
func TestOverflowBottomSheet_AnchoredToBottomOnMobile(t *testing.T) {
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
	if r.Min.X != dv.Bounds.Min.X || r.Max.X != dv.Bounds.Max.X {
		t.Fatalf("expected full-width popup [%d,%d]; got [%d,%d]", dv.Bounds.Min.X, dv.Bounds.Max.X, r.Min.X, r.Max.X)
	}
	if r.Max.Y != dv.Bounds.Max.Y {
		t.Fatalf("expected bottom-anchored popup (Max.Y=%d); got %d", dv.Bounds.Max.Y, r.Max.Y)
	}
}

// TestOverflowBottomSheet_DropdownOnDesktop verifies the desktop layout
// keeps the small floating dropdown anchored to the kebab (a full-width
// bottom sheet would waste the available chrome surface on mouse-driven
// desktop).
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

// TestOverflowBottomSheet_HasMinTouchHeight verifies the bottom sheet
// allocates at least TouchMinTarget() — never collapses below the touch
// floor even when the drum pane is short and items overflow.
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
