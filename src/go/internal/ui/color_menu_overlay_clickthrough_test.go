package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Reproducer for click-through bug: when the color dropdown overlaps the
// Upload button, selecting a swatch must not also trigger Upload. Previously,
// swatch clicks would close the menu and a still-held mouse press could fire
// the underlying Upload button on the following Update.
func TestColorMenuOverUploadDoesNotClickThrough(t *testing.T) {
	g := New(testLogger)
	g.Layout(800, 700)

	// Make drum pane small enough to force the color menu to open upward,
	// towards the control panel area where Upload lives.
	// Set bounds directly to avoid depending on splitter input.
	// Keep the drum view short so the color menu opens upward towards the
	// control panel. Use the current header size plus ~3 rows to avoid
	// depending on a hardcoded timelineHeight.
	h := timelineHeight + 3*g.drum.rowHeight()
	g.drum.SetBounds(image.Rect(0, g.split.Y, 800, g.split.Y+h))
	g.drum.recalcButtons()
	g.drum.calcLayout()

	// Open the color menu for row 0 (via button click) and ensure wheel exists.
	if len(g.drum.rowColorBtns) == 0 {
		t.Fatalf("no rowColorBtns")
	}
	g.drum.rowColorBtns[0].OnClick()
	g.drum.Update()
	if !g.drum.colorMenuOpen {
		t.Fatalf("color menu not open")
	}
	if g.drum.colorWheelRect.Empty() {
		t.Fatalf("color wheel not built")
	}

	// Pick a swatch near the top of the menu (opens upward) so it is likely
	// to overlap the Upload region. Index 2 (3rd) satisfies timeline math.
	r := g.drum.colorWheelRect
	cx, cy := r.Min.X+r.Dx()*2/3, r.Min.Y+r.Dy()/2

	// Hold mouse pressed across two updates to emulate the scenario where
	// the swatch click closes the menu, then the same held press could fire
	// the underlying Upload button on the next frame.
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 700 },
	)
	// First update: click swatch (menu handles and closes)
	_ = g.Update()
	// Second update: still held; without suppression this used to trigger Upload
	_ = g.Update()
	restore()

	if g.drum.uploading {
		t.Fatalf("click-through: upload triggered by color swatch click")
	}
}
