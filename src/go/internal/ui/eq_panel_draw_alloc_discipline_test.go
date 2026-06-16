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
// against the measured baseline + ~30% headroom so the test passes
// today AND catches a 2× regression. Tightening below baseline requires
// reducing real allocations first, NOT lowering the cap. Per-baseline
// history lives in the comments inside the map.
var perTabAllocBudget = map[PanelTab]float64{
	// Re-baselined twice: (1) after the gap audit fixed the harness to
	// re-Layout per active tab (tab-built content — Synth knob grid, Chain
	// stage cards — was previously never constructed, so Synth/Chain/Sampler
	// measured an EMPTY ~79-alloc draw and their caps were fiction); and
	// (2) after the draw-primitive alloc pass (packRGBA no longer boxes via
	// color.RGBAModel.Convert, drawRect reuses a shared DrawImageOptions,
	// and the Spectrum solid dB gridlines collapsed from per-pixel rects to
	// one full-width rect each — see drawrect_zero_alloc_test.go). That pass
	// roughly halved every tab and cut Spectrum 7010 → 1012. Baselines below
	// are the honest post-pass numbers (3 rows × 4 nodes soak scene,
	// 1280×720, 9-section Synth tab incl. the Phase-8C PITCH/LFO/BURST
	// cards). Caps = baseline × 1.3, rounded up.
	TabMeters: 95,  // honest baseline 73 (was 142 pre-pass). Still the tightest cap — the long-session OOM path.
	TabWave:   630, // baseline 481 (was 893 pre-pass)
	// The high-resolution FFT curve underlay (drawSpectrumCurve in
	// render_spectrum.go) batches adjacent equal-y columns into wide rects
	// and reuses spectrumCurveColScratch; with the per-pixel solid dB
	// gridlines fixed and drawRect alloc-free, the whole tab sits near the
	// other heavy tabs instead of 4× above them.
	TabSpectrum: 1320, // baseline 1012 (was 1763 → 7010 after Phase 1 curve → 1012 post-pass)
	TabEQ:       180,  // baseline 136 (was 213 pre-pass)
	// TabScope (Chain): 6 stage cards + meters + A/B trace.
	TabScope: 740, // baseline 566 (was 1087 pre-pass)
	// TabSynth: header + pipeline chip strip (9 chips + connector wires) +
	// ONE expanded detail pane (title/subtitle/pill + the selected stage's
	// knob captions). Re-baselined to 536 after the caption-readability pass:
	// every knob now resolves a human display name (prefix-strip + Title-case
	// prettifier) and formats its value through formatParamValue (SI-Hz,
	// signed units) every frame, replacing the old raw-id/bare-number labels.
	// Those per-knob string builds are the bulk of the new allocs. FOLLOW-UP:
	// cache the formatted caption per (paramID,value) so an idle editor tab
	// stops re-formatting unchanged labels — that would return this toward the
	// old ~239 baseline. Cap = baseline × 1.3.
	//
	// Re-baselined 536 -> 921 after the synthwave preview fills: the OSC and
	// ADSR mini-graphs now paint a per-column "outrun" wash beneath each curve
	// (one fill drawRect per plot column; drawRect boxes its color arg, so each
	// adds one alloc). The fill colors are fixed package vars served from
	// pixelCache (no per-column COLOR alloc), but the extra rects themselves
	// are the cost. Cap = baseline 921 x 1.3 ~= 1198.
	TabSynth: 1198, // baseline 921 (was 536 pre-synthwave-fill; 239 pre-caption-pass)
	// TabSampler: honest NO-SAMPLE baseline 100 (header + banner). The
	// loaded-sample waveform trace (per-column rects, like Wave) is not
	// exercised by this stubbed harness; keep Wave-magnitude headroom for it
	// rather than capping at the empty path.
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
			// Re-layout for the now-active tab. Without this, tab-built
			// content (the Synth tab's section cards + knob grid, the
			// Sampler trace) is never constructed and the measurement
			// covers an EMPTY draw (~79 allocs) instead of the real
			// render path — which is how the TabSynth budget sat at a
			// meaningless "generous initial cap" until the gap audit.
			g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
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
