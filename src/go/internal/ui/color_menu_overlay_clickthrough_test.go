package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A swatch pick over the Upload button must not also fire Upload on the still-held press.
func TestColorMenuOverUploadDoesNotClickThrough(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 700)

	// Short enough that the menu opens upward toward Upload, from the header plus ~3 rows.
	h := timelineHeight + 3*g.drum.rowHeight()
	g.drum.SetBounds(image.Rect(0, g.split.Y, 800, g.split.Y+h))
	g.drum.recalcButtons()
	g.drum.calcLayout()

	// Open the color menu for row 0 (via button click) and ensure wheel exists.
	if len(g.drum.rowColorBtns()) == 0 {
		t.Fatalf("no rowColorBtns")
	}
	g.drum.rowColorBtns()[0].OnClick()
	g.drum.Update()
	if !g.drum.IsColorMenuOpen() {
		t.Fatalf("color menu not open")
	}
	if g.drum.colorWheelComp.WheelRect().Empty() {
		t.Fatalf("color picker not built")
	}

	// Near the top of the upward menu, likely over Upload.
	r := g.drum.colorWheelComp.WheelRect()
	cx, cy := r.Min.X+r.Dx()*2/3, r.Min.Y+r.Dy()/2

	// Held across two updates: the pick closes the menu, and the same press must not reach Upload next frame.
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 700 },
	)
	t.Cleanup(restore)
	// First update: click swatch (menu handles and closes)
	_ = g.Update()
	_ = g.Update()
	restore()

	if g.drum.uploading {
		t.Fatalf("click-through: upload triggered by color swatch click")
	}
}
