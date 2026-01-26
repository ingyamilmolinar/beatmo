package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// Wheel scrolling should scroll rows, not zoom length.
func TestWheelScrollDoesNotChangeLength(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 400), nil, logger)
	// Add rows to make scrolling meaningful.
	for i := 0; i < 20; i++ {
		dv.AddRow()
	}
	origLen := dv.Length

	// Position cursor over steps area and simulate wheel down (negative wy).
	restore := SetInputForTest(
		func() (int, int) {
			return dv.Bounds.Min.X + dv.labelW + dv.controlsW + 10, dv.Bounds.Min.Y + dv.headerH + 10
		},
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, -1 }, // one notch downward
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)
	defer restore()

	prevOffset := dv.rowOffset
	dv.Update()
	if dv.Length != origLen {
		t.Fatalf("wheel scroll should not change length: before=%d after=%d", origLen, dv.Length)
	}
	if !(dv.rowOffset > prevOffset) {
		t.Fatalf("wheel scroll should advance rowOffset: before=%d after=%d", prevOffset, dv.rowOffset)
	}
}
