package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Ensure widgets retain minimum heights after repeated resizes.
func TestWidgetMinHeightsAfterRepeatedResizes(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 1280, 720), nil, logger)

	for i := 0; i < 5; i++ {
		dv.widgets.ResizeAxis("row", 1, 40)
		dv.widgets.ResizeAxis("row", 1, -60)
		dv.widgets.ResizeAxis("col", 0, 30)
		dv.widgets.ResizeAxis("col", 0, -50)
		dv.refreshWidgetLayout()
	}
	// Verify every row span keeps positive height.
	for r := 0; r < len(dv.widgets.rowPos)-1; r++ {
		h := dv.widgets.rowPos[r+1] - dv.widgets.rowPos[r]
		if h <= 8 {
			t.Fatalf("row %d collapsed to height %d", r, h)
		}
	}
}
