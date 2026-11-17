package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestEQBandColumns(t *testing.T) {
	assertDefaultParityState(t)
	prev := eqPanelHeight
	eqPanelHeight = 80
	defer func() { eqPanelHeight = prev }()

	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 300), nil, logger)
	dv.eqWaveformMode = false

	spec := make([]float64, 128)
	for i, b := range eqBandDefs {
		nq := 24000.0
		center := (b.loHz + b.hiHz) / 2
		bin := int(float64(len(spec)) * (center / nq))
		if bin >= len(spec) {
			bin = len(spec) - 1
		}
		spec[bin] = 1.0
		if i%2 == 1 {
			spec[bin] = 0.6
		}
	}
	snap := audio.AnalyzerSnapshot{RMS: 0.4, Spectrum: spec}
	dv.eqTestSnapshot = &snap

	dst := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	dv.drawEQ(dst)

	if got := len(dv.eqLastBands); got != len(eqBandDefs) {
		t.Fatalf("expected %d bands, got %d", len(eqBandDefs), got)
	}
	for i, v := range dv.eqLastBands {
		if v < 0.25 {
			t.Errorf("band %d too low, got %.3f", i, v)
		}
	}
}
