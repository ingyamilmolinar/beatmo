//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// DISTINCT per-stage snapshots, so a renderer that drops trace B fails; the same sine in every slot would pass.

func TestChainPanelRendersBothTracesForPerInstrumentSnapshots(t *testing.T) {
	assertDefaultParityState(t)
	// The cache renders at a (0,0) origin, so exercise the direct path to count rects on screen.
	defer SetChainTraceCacheForTest(false)()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	if err := RunScene(g, "crop_chain_with_two_stages"); err != nil {
		t.Fatalf("RunScene: %v", err)
	}
	for i := 0; i < 32; i++ {
		_ = g.Update()
	}

	// Different period and amplitude, so one slot bleeding into the other is caught.
	synthSnap := audio.AnalyzerSnapshot{
		RMS:  0.42,
		Peak: 0.6,
		Waveform: func() []float64 {
			out := make([]float64, 512)
			for i := range out {
				out[i] = 0.6 * math.Sin(2*math.Pi*float64(i)/64.0)
			}
			return out
		}(),
		Spectrum: []float64{0.5, 0.4, 0.3, 0.2, 0.1},
	}
	preEQSnap := audio.AnalyzerSnapshot{
		RMS:  0.21,
		Peak: 0.3,
		Waveform: func() []float64 {
			out := make([]float64, 512)
			for i := range out {
				out[i] = 0.3 * math.Sin(2*math.Pi*float64(i)/32.0)
			}
			return out
		}(),
		Spectrum: []float64{0.25, 0.2, 0.15, 0.1, 0.05},
	}

	// As a wired WASM bridge produces for one instrument: only the two selected stages carry data.
	state := SynthesizeScopeState(channel.Of(roster.Kick), scope.StageSynth, scope.StageInsertFX, ScopeStageSnapshots{
		Synth:  synthSnap,
		PreEQ:  preEQSnap,
		PostEQ: audio.AnalyzerSnapshot{},
		Master: audio.AnalyzerSnapshot{},
	})
	if !state.TapA.Active {
		t.Fatalf("synthesized TapA not active: %+v", state.TapA)
	}
	if !state.TapB.Active {
		t.Fatalf("synthesized TapB not active: %+v", state.TapB)
	}
	testScopeStateOverride = state
	t.Cleanup(func() { testScopeStateOverride = nil })

	// Make sure tap selection matches what the override claims.
	if z := chainZoneOf(g); z != nil {
		z.SetTapA(scope.StageSynth)
		z.SetTapB(scope.StageInsertFX)
		z.SetTraceVisible("A", true)
		z.SetTraceVisible("B", true)
	}

	rect, ok := g.SubjectRect(SubjectChain)
	if !ok || rect.Empty() {
		t.Fatalf("SubjectRect(chain) ok=%v rect=%v", ok, rect)
	}

	screen := ebiten.NewImage(1280, 720)
	rects := collectFilledRects(t, func() {
		g.Draw(screen)
	})

	nA := rectsWithColorInside(rects, rect, colScopeA)
	nB := rectsWithColorInside(rects, rect, colScopeB)
	if nA == 0 {
		t.Errorf("trace A (colScopeA) drew %d rects inside chain SubjectRect %v — want > 0", nA, rect)
	}
	if nB == 0 {
		t.Errorf("trace B (colScopeB) drew %d rects inside chain SubjectRect %v — want > 0", nB, rect)
	}

	// A fill per column from the trace to the zero line, beneath the crisp line.
	if fA := rectsWithColorInside(rects, rect, colScopeAFill); fA == 0 {
		t.Errorf("trace A synthwave fill (colScopeAFill) drew %d rects in %v — want > 0", fA, rect)
	}
	if fB := rectsWithColorInside(rects, rect, colScopeBFill); fB == 0 {
		t.Errorf("trace B synthwave fill (colScopeBFill) drew %d rects in %v — want > 0", fB, rect)
	}
	if testing.Verbose() {
		t.Logf("chain rect=%v colScopeA=%d colScopeB=%d", rect, nA, nB)
	}
}
