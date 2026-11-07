package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The dispatcher moves the splitter mid-frame while the grid pane reads it live, so drum bounds baked from the
// previous frame open a dark band behind a fast drag; the drum's top edge must equal the live splitter.
func TestSplitterDragKeepsDrumBoundsInSync(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	pressed := false
	mx, my := 0, 0
	cur := func() (int, int) { return mx, my }
	mouse := func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft }
	restore := SetInputForTest(
		cur, mouse,
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	// Settle one frame so the drum bounds initialize from the splitter.
	_ = g.Update()

	startY := g.split.Y
	if startY <= 200 {
		t.Fatalf("unexpected starting splitter Y %d", startY)
	}

	// Press on the divider handle (the centered pill).
	mx, my = 400, startY
	pressed = true
	_ = g.Update() // press frame: drag arms, Y not yet moved

	// Fast drag upward by 80px in a single frame (the worst case for the gap).
	targetY := startY - 80
	mx, my = 400, targetY
	_ = g.Update() // move frame: dispatcher sets s.Y = targetY

	if !g.split.dragging {
		t.Fatalf("expected splitter to be dragging after press on the handle")
	}
	if got, want := g.drum.Bounds.Min.Y, g.split.Y; got != want {
		t.Fatalf("drum top %d lags live splitter Y %d during drag: %d px dark band",
			got, want, want-got)
	}
}
