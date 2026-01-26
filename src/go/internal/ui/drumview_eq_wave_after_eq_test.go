package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// Regression: waveform/analyzer should remain live after applying EQ.
func TestEQWaveformAfterEQ(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 320), nil, logger)
	// Apply a small EQ change.
	dv.eqBandGainsDB = make([]float64, len(eqBandDefs))
	dv.eqBandGainsDB[0] = 3
	dv.applyEQ()

	snap := dv.analyzerSnapshot()
	if len(snap.Spectrum) == 0 && len(snap.Waveform) == 0 {
		t.Fatalf("expected analyzer snapshot after EQ to have data")
	}
}
