package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestGridHelpButtonClickThroughUpdate drives the REAL Game.Update() input loop
// with a simulated left-click at the grid help button's center and asserts the
// shortcuts overlay opens. This reproduces the user-visible "clicking does
// nothing" path that the isolated HandleInputResult test cannot catch.
func TestGridHelpButtonClickThroughUpdate(t *testing.T) {
	g := newTestGameForUndo(t)
	if g.gridHelpBtn == nil {
		t.Fatal("grid help button should be constructed")
	}
	r := g.gridHelpButtonRect()
	if r.Empty() {
		t.Fatal("grid help button rect empty on desktop")
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2

	// Match the screen size the test game was laid out at (800x600) so Update
	// doesn't relayout the button rect out from under our click coordinates.
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	// Press frame: HandleInputResult fires OnClick on the press edge.
	g.Update()

	if !g.drum.portal().IsOpen() {
		t.Fatal("left-click on the grid help button (via real Update) should open the shortcuts overlay")
	}
}
