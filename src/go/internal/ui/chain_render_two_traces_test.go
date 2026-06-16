//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Regression guard for the Chn-tab "only one trace renders" bug: feed the
// renderer DISTINCT per-stage snapshots (Synth ≠ PreEQ) for a per-instrument
// id, then assert BOTH colScopeA and colScopeB rect fills land inside the
// chain SubjectRect. The existing audio_panel_render_pixels_test.go feeds
// the same sine into every stage slot, so even a renderer that silently
// dropped trace B would pass.

func TestChainPanelRendersBothTracesForPerInstrumentSnapshots(t *testing.T) {
	assertDefaultParityState(t)
	// This test counts trace rects at their on-screen position via the
	// drawRect interceptor; the production trace cache renders them at a
	// (0,0) origin before blitting, so exercise the direct render path.
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

	// Synth snapshot: a 64-period sine. PreEQ snapshot: a 32-period
	// half-amplitude sine. Distinct enough that even a partial-render
	// regression (e.g. one slot bleeding into the other) would be caught.
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

	// Build a ScopeStateOverride simulating what a fully-wired WASM bridge
	// would produce for a per-instrument id: only the two selected stages
	// carry data. All other slots are zero.
	state := SynthesizeScopeState("kick", scope.StageSynth, scope.StageInsertFX, ScopeStageSnapshots{
		Synth:  synthSnap,
		PreEQ:  preEQSnap,
		PostEQ: audio.AnalyzerSnapshot{},
		Sends:  audio.AnalyzerSnapshot{},
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

	// Synthwave "outrun" fills must be painted beneath each trace (the
	// per-lane trace color at sub-full AlphaSubtle), one fill rect per
	// column from the trace to the zero-line — crisp line stays on top.
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
