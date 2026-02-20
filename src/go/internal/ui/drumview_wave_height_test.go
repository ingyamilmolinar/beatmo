package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Ensure the Wave/EQ widget starts near one-third of the drum view height.
func TestWaveDefaultHeightOneThird(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 1200, 720), nil, logger)
	h := dv.Bounds.Dy()
	waveH := dv.widgetRects[WidgetWave].Dy()
	wantMin := h / 3
	if waveH < wantMin-10 { // small tolerance
		t.Fatalf("wave height too small: got=%d want>=%d (drumH=%d)", waveH, wantMin-10, h)
	}
}
