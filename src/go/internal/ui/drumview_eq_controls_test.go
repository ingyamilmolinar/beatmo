package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestEQControlSlidersApplyGains(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 320), nil, logger)
	dv.eqWaveformMode = false
	// Simulate one draw to ensure bands allocated.
	dv.calcLayout()
	if len(dv.eqBandGainsDB()) != len(eqBandDefs) {
		t.Fatalf("expected %d EQ bands, got %d", len(eqBandDefs), len(dv.eqBandGainsDB()))
	}
	// Boost first band.
	dv.eqBandGainsDB()[0] = 12
	dv.applyEQ()
	rec := audio.LastSetEQ()
	if rec.ID != "main" {
		t.Fatalf("expected main EQ to be applied, got id=%s", rec.ID)
	}
	if len(rec.Bands) != len(eqBandDefs) {
		t.Fatalf("expected %d bands applied, got %d", len(eqBandDefs), len(rec.Bands))
	}
	if rec.Bands[0].GainDB <= 0 {
		t.Fatalf("expected first band gain > 0, got %.2f", rec.Bands[0].GainDB)
	}
	// Cut high band.
	last := len(dv.eqBandGainsDB()) - 1
	dv.eqBandGainsDB()[last] = -12
	dv.applyEQ()
	rec = audio.LastSetEQ()
	if rec.Bands[last].GainDB >= 0 {
		t.Fatalf("expected last band negative gain, got %.2f", rec.Bands[last].GainDB)
	}
}
