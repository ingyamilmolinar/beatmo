//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// TestChainTriggerMarker_PaintsLine: the trigger marker must paint a
// vertical line at ~10% of the trace area's width. We verify via the
// shared drawRect interceptor.
func TestChainTriggerMarker_PaintsLine(t *testing.T) {
	dst := ebiten.NewImage(400, 100)
	rect := image.Rect(0, 0, 400, 80)
	rects := collectFilledRects(t, func() {
		drawChainTriggerMarker(dst, rect)
	})
	if len(rects) == 0 {
		t.Fatalf("expected trigger marker rects, got 0")
	}
	wantX := rect.Min.X + rect.Dx()/10
	var seen bool
	for _, r := range rects {
		// Tolerance ±2 px so a small geometric refactor doesn't break us.
		if r.Rect.Min.X >= wantX-2 && r.Rect.Min.X <= wantX+2 {
			seen = true
			break
		}
	}
	if !seen {
		t.Errorf("expected a rect near x=%d (10%% mark), got rects=%v", wantX, rects)
	}
}

// TestChainStageMiniWaveform_NoSamplesIsNoop: empty samples slice
// must not draw anything.
func TestChainStageMiniWaveform_NoSamplesIsNoop(t *testing.T) {
	dst := ebiten.NewImage(100, 20)
	rects := collectFilledRects(t, func() {
		drawChainStageMiniWaveform(dst, image.Rect(0, 0, 50, 10), nil, true)
	})
	if len(rects) != 0 {
		t.Errorf("expected 0 rects for empty samples, got %d", len(rects))
	}
}

// TestChainStageMiniWaveform_DrawsForActiveSamples: a populated samples
// slice must produce at least one filled rect inside the strip.
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

// TestChainFXBadgeCount_ShownWhenPositive: a positive count produces
// at least one rect (the badge background).
func TestChainFXBadgeCount_ShownWhenPositive(t *testing.T) {
	dst := ebiten.NewImage(100, 30)
	rects := collectFilledRects(t, func() {
		drawChainFXBadgeCount(dst, image.Rect(0, 0, 50, 22), 3)
	})
	if len(rects) == 0 {
		t.Errorf("expected at least one rect for count=3, got 0")
	}
}

// TestChainInsertFXCountForActiveRow_RouterEmpty: with no active taps
// the count returns 0, never calls audio.GetInsertEffects with empty
// id.
func TestChainInsertFXCountForActiveRow_NoTaps(t *testing.T) {
	if got := chainInsertFXCountForActiveRow(nil); got != 0 {
		t.Errorf("nil state: got %d want 0", got)
	}
	state := &scope.State{}
	if got := chainInsertFXCountForActiveRow(state); got != 0 {
		t.Errorf("inactive taps: got %d want 0", got)
	}
}

// TestChainInsertFXCountForActiveRow_RoutesToAudioGetInsertEffects:
// with an active TapA carrying an instID, the helper must consult
// audio.GetInsertEffects. We add a real insert effect to verify the
// count is non-zero.
func TestChainInsertFXCountForActiveRow_ReadsAudioLayer(t *testing.T) {
	const id = "chain-fxbadge-test"
	// Add a delay effect via the audio API so GetInsertEffects returns
	// a non-empty slice. This exercises the full path through the
	// effect-chain manager.
	if slot := audio.AddInsertEffect(id, audio.EffectDelay, nil); slot < 0 {
		t.Fatalf("AddInsertEffect returned negative slot: %d", slot)
	}
	defer audio.SetInsertEffects(id, nil)

	state := &scope.State{
		TapA: scope.TapData{
			Stage:  scope.StageInsertFX,
			InstID: id,
			Active: true,
		},
	}
	if got := chainInsertFXCountForActiveRow(state); got < 1 {
		t.Errorf("expected at least 1 active insert effect, got %d", got)
	}
}

// TestChainMiniWaveHeightScalesWithDensity — the per-stage mini-waveform
// strip is density-driven (was a fixed 6px). At Spacious the painted trace
// must span a taller region than at Compact, proving the token is consumed.
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
