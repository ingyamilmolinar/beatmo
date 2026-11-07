//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// A hot reading lights a meter-coloured bar beside that stage.
func TestChainPhase3_PerStageMeterLitForActiveStage(t *testing.T) {
	cb := ChainCallbacks{
		ScopeState: func() *scope.State { return nil },
		StagePeak: func(stage scope.Stage) (float64, float64) {
			if stage == scope.StageEQ {
				return -0.5, -9 // hot enough to land in the red zone (> meterRedDB)
			}
			return -200, -200 // silent
		},
	}
	z := NewChainPanelZone(cb)
	z.Layout(image.Rect(0, 0, 600, 200))
	dst := ebiten.NewImage(600, 200)
	rects := collectFilledRects(t, func() { z.Draw(dst) })

	// -0.5 is above meterRedDB (-1), so meterHigh.
	col := image.Rect(0, 0, chainStageColW+16, 200)
	if got := rectsWithColorInside(rects, col, meterHigh); got == 0 {
		t.Fatalf("expected ≥1 meterHigh rect (per-stage meter) in stage column %v, got 0", col)
	}
}

// Silent stages stay dark. meterLow shares the accent gold and appears in chrome; meterMid and meterHigh are
// distinct, so only they are checked.
func TestChainPhase3_PerStageMeterDarkForSilentStage(t *testing.T) {
	cb := ChainCallbacks{
		ScopeState: func() *scope.State { return nil },
		StagePeak: func(stage scope.Stage) (float64, float64) {
			// All stages silent.
			return -200, -200
		},
	}
	z := NewChainPanelZone(cb)
	z.Layout(image.Rect(0, 0, 600, 200))
	dst := ebiten.NewImage(600, 200)
	rects := collectFilledRects(t, func() { z.Draw(dst) })

	col := image.Rect(0, 0, chainStageColW+16, 200)
	gotRed := rectsWithColorInside(rects, col, meterHigh)
	gotYel := rectsWithColorInside(rects, col, meterMid)
	// meterLow == primary accent (#FFB30A) — skip; appears in chrome at all times.
	if gotRed+gotYel > 0 {
		t.Fatalf("expected no meterMid/meterHigh rects in silent state, got orange=%d red=%d", gotYel, gotRed)
	}
}

func TestChainPhase3_NilCallbackTolerated(t *testing.T) {
	cb := ChainCallbacks{
		ScopeState: func() *scope.State { return nil },
	}
	z := NewChainPanelZone(cb)
	z.Layout(image.Rect(0, 0, 600, 200))
	dst := ebiten.NewImage(600, 200)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Chain panel Draw panicked without StagePeak callback: %v", r)
		}
	}()
	z.Draw(dst)
}
