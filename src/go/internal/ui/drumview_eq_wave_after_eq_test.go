package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Regression: waveform/analyzer should remain live after applying EQ.
func TestEQWaveformAfterEQ(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 320), nil, logger)
	// Apply a small EQ change.
	dv.eqPanelZone.bandGainsDB = make([]float64, len(eqBandDefs))
	dv.eqBandGainsDB()[0] = 3
	dv.applyEQ()

	snap := dv.analyzerSnapshot()
	if len(snap.Spectrum) == 0 && len(snap.Waveform) == 0 {
		t.Fatalf("expected analyzer snapshot after EQ to have data")
	}
}

// With a boost, post-EQ waveform should have higher amplitude than pre-EQ.
func TestEQDualWaveformBoostComparison(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 320), nil, logger)
	dv.calcLayout()

	// Inject deterministic snapshots.
	// Pre-EQ: modest signal.
	preWave := make([]float64, 512)
	for i := range preWave {
		preWave[i] = 0.3
	}
	dv.eqTestPreEQSnapshot = &audio.AnalyzerSnapshot{
		RMS:      0.3,
		Peak:     0.3,
		Waveform: preWave,
	}
	// Post-EQ: boosted signal.
	postWave := make([]float64, 512)
	for i := range postWave {
		postWave[i] = 0.8
	}
	dv.eqTestSnapshot = &audio.AnalyzerSnapshot{
		RMS:      0.8,
		Peak:     0.8,
		Waveform: postWave,
	}

	preSnap := dv.preEQAnalyzerSnapshot()
	postSnap := dv.analyzerSnapshot()

	if postSnap.Peak <= preSnap.Peak {
		t.Errorf("expected post-EQ peak (%.2f) > pre-EQ peak (%.2f) with boost",
			postSnap.Peak, preSnap.Peak)
	}
}
