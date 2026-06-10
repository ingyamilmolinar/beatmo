//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// TestChainPhase3_PerStageMeterLitForActiveStage — when StagePeak
// returns a hot reading for a stage, the Chain panel must paint a
// meterColor()-tinted vertical bar beside that stage's button.
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

	// meterColor(-0.5) = meterRed (since -0.5 > meterRedDB=-1).
	// Look for at least one meterRed rect inside the stage column area
	// (left chainStageColW pixels of the panel).
	col := image.Rect(0, 0, chainStageColW+16, 200)
	if got := rectsWithColorInside(rects, col, meterRed); got == 0 {
		t.Fatalf("expected ≥1 meterRed rect (per-stage meter) in stage column %v, got 0", col)
	}
}

// TestChainPhase3_PerStageMeterDarkForSilentStage — silent stages must
// NOT light up. The whole point of the per-stage display is "this
// stage is active right now" vs "this stage is silent right now".
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
	gotRed := rectsWithColorInside(rects, col, meterRed)
	gotYel := rectsWithColorInside(rects, col, meterYellow)
	gotGreen := rectsWithColorInside(rects, col, meterGreen)
	if gotRed+gotYel+gotGreen > 0 {
		t.Fatalf("expected no meter-color rects in silent state, got red=%d yel=%d green=%d", gotRed, gotYel, gotGreen)
	}
}

// TestChainPhase3_NilCallbackTolerated — when StagePeak is nil
// (legacy embed contexts), the renderer must not panic.
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
