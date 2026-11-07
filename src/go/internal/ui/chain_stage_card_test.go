//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

func TestChainStageMiniWaveform_NoSamplesIsNoop(t *testing.T) {
	dst := ebiten.NewImage(100, 20)
	rects := collectFilledRects(t, func() {
		drawChainStageMiniWaveform(dst, image.Rect(0, 0, 50, 10), nil, true)
	})
	if len(rects) != 0 {
		t.Errorf("expected 0 rects for empty samples, got %d", len(rects))
	}
}

func TestChainStageMiniWaveform_DrawsForActiveSamples(t *testing.T) {
	dst := ebiten.NewImage(100, 20)
	samples := make([]float64, 256)
	for i := range samples {
		samples[i] = 0.5 // constant positive amplitude
	}
	rect := image.Rect(0, 0, 50, 10)
	rects := collectFilledRects(t, func() {
		drawChainStageMiniWaveform(dst, rect, samples, true)
	})
	if len(rects) == 0 {
		t.Errorf("expected mini-waveform rects, got 0")
	}
	for _, r := range rects {
		if !r.Rect.In(rect) {
			t.Errorf("waveform rect %v not inside strip %v", r.Rect, rect)
			break
		}
	}
}

// TestChainFXBadgeCount_HiddenWhenZero: count=0 produces no draw.
func TestChainFXBadgeCount_HiddenWhenZero(t *testing.T) {
	dst := ebiten.NewImage(100, 30)
	rects := collectFilledRects(t, func() {
		drawChainFXBadgeCount(dst, image.Rect(0, 0, 50, 22), 0)
	})
	if len(rects) != 0 {
		t.Errorf("expected 0 rects for count=0, got %d", len(rects))
	}
}

func TestChainFXBadgeCount_ShownWhenPositive(t *testing.T) {
	dst := ebiten.NewImage(100, 30)
	rects := collectFilledRects(t, func() {
		drawChainFXBadgeCount(dst, image.Rect(0, 0, 50, 22), 3)
	})
	if len(rects) == 0 {
		t.Errorf("expected at least one rect for count=3, got 0")
	}
}

// 0 with no taps, never asking audio.GetInsertEffects for an empty id.
func TestChainInsertFXCountForActiveRow_NoTaps(t *testing.T) {
	if got := chainInsertFXCountForActiveRow(nil); got != 0 {
		t.Errorf("nil state: got %d want 0", got)
	}
	state := &scope.State{}
	if got := chainInsertFXCountForActiveRow(state); got != 0 {
		t.Errorf("inactive taps: got %d want 0", got)
	}
}

// A real insert effect, so the count comes from the audio layer.
func TestChainInsertFXCountForActiveRow_ReadsAudioLayer(t *testing.T) {
	const id = "chain-fxbadge-test"
	if slot := audio.AddInsertEffect(id, audio.EffectDelay, nil); slot < 0 {
		t.Fatalf("AddInsertEffect returned negative slot: %d", slot)
	}
	defer audio.SetInsertEffects(id, nil)

	state := &scope.State{
		TapA: scope.TapData{
			Stage:  scope.StageInsertFX,
			InstID: channel.Of(id),
			Active: true,
		},
	}
	if got := chainInsertFXCountForActiveRow(state); got < 1 {
		t.Errorf("expected at least 1 active insert effect, got %d", got)
	}
}

// Density-driven, so Spacious spans a taller strip than Compact.
func TestChainMiniWaveHeightScalesWithDensity(t *testing.T) {
	samples := make([]float64, 200)
	for i := range samples {
		if i%2 == 0 {
			samples[i] = 0.9
		} else {
			samples[i] = -0.9
		}
	}
	st := &scope.State{TapA: scope.TapData{Stage: scope.StageSynth, Samples: samples, Active: true}}

	traceSpan := func(d Density) int {
		restore := SetDensityForTest(d)
		defer restore()
		z := NewChainPanelZone(ChainCallbacks{ScopeState: func() *scope.State { return st }})
		z.SetTapA(scope.StageSynth)
		z.Layout(image.Rect(0, 0, 600, 240))
		dst := ebiten.NewImage(600, 240)
		traceCol := color.RGBAModel.Convert(WithAlpha(genColorVizScopeTraceA, AlphaStrong)).(color.RGBA)
		minY, maxY := 1<<30, -(1 << 30)
		rects := collectFilledRects(t, func() {
			z.drawChainStageCardDecorations(dst, z.stageButtons[0], 0, true, false)
		})
		for _, r := range rects {
			if r.Color == traceCol {
				if r.Rect.Min.Y < minY {
					minY = r.Rect.Min.Y
				}
				if r.Rect.Max.Y > maxY {
					maxY = r.Rect.Max.Y
				}
			}
		}
		if maxY < minY {
			return 0
		}
		return maxY - minY
	}

	compact := traceSpan(DensityCompact)
	spacious := traceSpan(DensitySpacious)
	if compact == 0 || spacious == 0 {
		t.Fatalf("mini-wave trace not found: compact=%d spacious=%d", compact, spacious)
	}
	if spacious <= compact {
		t.Errorf("mini-wave strip should be taller at Spacious: compact=%d spacious=%d", compact, spacious)
	}
}
