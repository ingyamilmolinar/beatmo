//go:build test

// 200 Draws per tab against a per-tab AllocsPerRun budget; a new tab extends the map. Generous, to catch
// order-of-magnitude regressions; Meters is tightest, the long-session OOM path.

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Measured baseline × 1.3, which catches a 2× regression. Lower a cap only after the allocations drop.
var perTabAllocBudget = map[PanelTab]float64{
	// Measured on the 3 rows × 4 nodes soak scene at 1280×720, re-laid-out per tab so tab-built content is real.
	TabMeters: 95,  // honest baseline 73 (was 142 pre-pass). Still the tightest cap — the long-session OOM path.
	TabWave:   630, // baseline 481 (was 893 pre-pass)
	// The static dB grid, ticks and labels live in a sprite; Spectrum has no data gate and pays on every frame.
	TabSpectrum: 229, // baseline 176
	TabEQ:       180, // baseline 136 (was 213 pre-pass)
	// TabScope (Chain): 6 stage cards + meters + A/B trace.
	TabScope: 740, // baseline 566 (was 1087 pre-pass)
	// The graph is one cached sprite of measured audio; its glow a baked layer.
	TabSynth: 102, // baseline 78 (921 with the model-drawn previews)
	// The stub harness has no loaded sample, so this keeps Wave-sized headroom for its trace.
	TabSampler: 800,
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

	// Pre-built states, so the renderer is measured, not callback churn.
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
				ID: channel.Master, Name: "Master", PeakDB: -6, RMSDB: -12, Active: true,
				FFTBins: fft, FreqBins: freq, Waveform: wave,
			},
		}
	}
	stubScalar := func() *analyzer.State {
		return &analyzer.State{
			Master: analyzer.ChannelMetrics{
				ID: channel.Master, Name: "Master", PeakDB: -6, RMSDB: -12, Active: true,
			},
		}
	}
	g.drum.eqPanelZone.callbacks.AnalyzerState = stubFull
	g.drum.eqPanelZone.callbacks.AnalyzerMetricsOnly = stubScalar
	g.drum.eqPanelZone.callbacks.DrawWaveform = func(dst *ebiten.Image) {}

	scratch := ebiten.NewImage(1280, 720)

	// So the first Draw's layout does not skew AllocsPerRun.
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
			// Per tab, or tab-built content is never constructed and an empty draw is measured.
			g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
			// So lazy init stays out of the measurement.
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
