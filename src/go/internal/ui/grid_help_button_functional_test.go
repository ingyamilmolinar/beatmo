package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Through the real Update loop: the isolated HandleInputResult test cannot see "clicking does nothing".
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

	// The layout size (800x600), so Update does not relayout the button under the click.
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
