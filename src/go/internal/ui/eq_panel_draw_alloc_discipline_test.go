//go:build test

// Phase 3B — per-tab Draw allocation discipline.
//
// For every PanelTab value, drive (*EQPanelZone).Draw 200 times under
// stubbed callbacks and assert testing.AllocsPerRun against a per-tab
// budget. Adding a new tab requires extending the budget map; new code
// that exceeds an existing tab's budget fails CI.
//
// Budgets are intentionally generous — they catch order-of-magnitude
// regressions, not micro-churn. The Meters tab is held tightest
// because that is the path the long-session OOM hit.

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// perTabAllocBudget caps allocations per Draw call. Numbers calibrated
// against the post-Phase-2A baseline + ~30% headroom so the test passes
// today AND catches a 2× regression. Tightening below baseline requires
// reducing real allocations first, NOT lowering the cap.
//
// Baseline measurements (post-Phase-2A wiring, 3 rows × 4 nodes scene,
// 1280x720 viewport):
//
//	Meters    82 allocs/Draw   — metrics-only path; mostly text glyphs + dB labels
//	Wave     893 allocs/Draw   — Waveform polyline draw + axis text
//	Spectrum 1763 allocs/Draw  — 10-band tick labels + peak-hold + bar text
//	EQ      1038 allocs/Draw   — 10 band sliders + dB readout text
//	Scope     50 allocs/Draw   — delegated to ChainPanelZone (mostly empty under stub)
//
// Budget = baseline × 1.3 (tight) for Meters, ×1.4 for the heavy tabs,
// rounded up. The tight Meters cap is intentional — that path was the
// OOM hotspot and any regression there is load-bearing.
var perTabAllocBudget = map[PanelTab]float64{
	TabMeters:   110,  // baseline 82 → cap 110 (35% headroom; OOM-affected path)
	TabWave:     1300, // baseline 893 → cap 1300
	TabSpectrum: 2400, // baseline 1763 → cap 2400
	TabEQ:       1400, // baseline 1038 → cap 1400
	TabScope:    100,  // baseline 50 → cap 100
}

func TestEQPanelDrawAllocDiscipline(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdivisions: %v", err)
	}
	buildSoakScene(t, g, 3, 4)

	if g.drum.eqPanelZone == nil {
		t.Fatal("eqPanelZone nil; harness assumption broken")
	}

	// Stub callbacks return pre-built non-nil states so we measure the
	// renderer cost, not callback churn. Spectrum/Wave/EQ all need
	// non-empty FFT/Wave fields to render; Meters only needs scalars.
	const fftLen = 64
	stubFull := func() *analyzer.State {
		fft := make([]float64, fftLen)
		freq := make([]float64, fftLen)
		wave := make([]float64, fftLen)
		for i := 0; i < fftLen; i++ {
			fft[i] = -float64(i)
			freq[i] = float64(i) * 100
			wave[i] = float64(i) / fftLen
		}
		return &analyzer.State{
			Master: analyzer.ChannelMetrics{
				ID: "main", Name: "Master", PeakDB: -6, RMSDB: -12, Active: true,
				FFTBins: fft, FreqBins: freq, Waveform: wave,
			},
		}
	}
	stubScalar := func() *analyzer.State {
		return &analyzer.State{
			Master: analyzer.ChannelMetrics{
				ID: "main", Name: "Master", PeakDB: -6, RMSDB: -12, Active: true,
			},
		}
	}
	g.drum.eqPanelZone.callbacks.AnalyzerState = stubFull
	g.drum.eqPanelZone.callbacks.AnalyzerMetricsOnly = stubScalar
	g.drum.eqPanelZone.callbacks.DrawWaveform = func(dst *ebiten.Image) {}

	scratch := ebiten.NewImage(1280, 720)

	// Force a layout so hit areas + button rects are populated; without
	// this the first Draw triggers extra layout allocations that skew
	// AllocsPerRun.
	g.drum.eqPanelZone.Draw(scratch)

	for _, tab := range AllPanelTabs() {
		budget, ok := perTabAllocBudget[tab]
		if !ok {
			t.Errorf("PanelTab %v has no entry in perTabAllocBudget — "+
				"add one with a documented rationale", tab)
			continue
		}
		t.Run(PanelTabLabel(tab), func(t *testing.T) {
			g.drum.eqPanelZone.tabState.SetActiveTab(tab)
			// Warm up so the first call's lazy init doesn't pollute
			// the measurement.
			for i := 0; i < 5; i++ {
				g.drum.eqPanelZone.Draw(scratch)
			}
			allocs := testing.AllocsPerRun(50, func() {
				g.drum.eqPanelZone.Draw(scratch)
			})
			t.Logf("tab=%v allocs/Draw = %.1f (budget %.0f)", tab, allocs, budget)
			if allocs > budget {
				t.Errorf("tab=%v allocs/Draw = %.1f exceeds budget %.0f — "+
					"new alloc-heavy code path on this tab; either fix the leak "+
					"or update perTabAllocBudget with rationale",
					tab, allocs, budget)
			}
		})
	}
}
