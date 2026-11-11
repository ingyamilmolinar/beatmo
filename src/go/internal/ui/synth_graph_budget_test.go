package ui

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/soundview"
)

// graphAnalysisBudget is what the analysis behind one graph render may cost
// per second of note (spec 2026-10-25 §3.4): on desktop it runs on the graph
// worker; in the browser it runs on the page's one thread in yielded slices,
// so its total is what a render takes away from the frames around it.
const graphAnalysisBudget = 15 * time.Millisecond

// TestGraphAnalysisStaysInBudget: for every view, the analyses it needs on a
// 1 s note (a decaying, gliding, noisy tone — every analysis has work to do)
// cost at most graphAnalysisBudget. The fastest of seven runs is judged, so a
// busy machine does not fail it; a real regression does.
func TestGraphAnalysisStaysInBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	if raceEnabled {
		t.Skip("the race detector slows these loops several-fold; the budget is for real code")
	}
	rng := rand.New(rand.NewSource(3))
	pcm := make([]float32, testViewSR)
	ph := 0.0
	for i := range pcm {
		tt := float64(i) / testViewSR
		ph += (60 + 200*math.Exp(-tt/0.03)) / testViewSR
		pcm[i] = float32(math.Exp(-tt/0.3)*math.Sin(2*math.Pi*ph) + 0.05*rng.NormFloat64())
	}
	for _, v := range allPlotViews {
		need := synthViewNeeds(v)
		best := time.Duration(math.MaxInt64)
		for run := 0; run < 7; run++ {
			start := time.Now()
			soundview.New(pcm, testViewSR, 1).Warm(need, nil)
			if d := time.Since(start); d < best {
				best = d
			}
		}
		t.Logf("%-24s %6.2f ms", synthViewTitle(v), float64(best)/1e6)
		if best > graphAnalysisBudget {
			t.Errorf("%s: its analysis of a 1 s note takes %v, over the %v budget", synthViewTitle(v), best, graphAnalysisBudget)
		}
	}
}

// TestViewsReadOnlyTheAnalysesTheyWarm: a view's plot is built on the UI
// goroutine (the browser's one thread) from analyses warmed on the worker
// (synthViewNeeds). A view that reads anything else computes it right there,
// in one unyielded stretch — so building any view, on a pitched note and on
// noise (where views fall back), must leave every unwarmed analysis unwarmed.
func TestViewsReadOnlyTheAnalysesTheyWarm(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	notes := map[string]func(i int) float32{
		"pitched": func(i int) float32 {
			tt := float64(i) / testViewSR
			return float32(0.6 * math.Exp(-tt/0.3) * math.Sin(2*math.Pi*220*tt))
		},
		"noise": func(i int) float32 {
			tt := float64(i) / testViewSR
			return float32(0.5 * math.Exp(-tt/0.1) * rng.NormFloat64())
		},
	}
	all := []soundview.Need{soundview.NeedEnvelope, soundview.NeedOnsets, soundview.NeedPitch, soundview.NeedSTFT, soundview.NeedHarmonics}
	for name, f := range notes {
		for _, v := range allPlotViews {
			mk := func(tag string) *graphRender {
				pcm := make([]float32, testViewSR/2)
				for i := range pcm {
					pcm[i] = f(i)
				}
				an := soundview.New(pcm, testViewSR, 0.5)
				r := &graphRender{ident: graphIdent{hash: name + tag}, sr: testViewSR, voiceSec: 0.5, an: an}
				// The worker's two rounds: the view's needs, then what this
				// render turns out to need (an unpitched close-up's colour).
				an.Warm(synthViewNeeds(v), nil)
				an.Warm(synthRenderNeeds(v, r), nil)
				return r
			}
			now, lo, before := mk("now"), mk("lo"), mk("before")
			buildSynthViewPlot(v, 420, 170, synthViewInput{now: now, lo: lo, before: before})
			for _, n := range all {
				if synthRenderNeeds(v, now)&n == 0 && (now.an.Has(n) || lo.an.Has(n) || before.an.Has(n)) {
					t.Errorf("%s note, %s: building the plot computed analysis %d it does not warm (synthViewNeeds)", name, synthViewTitle(v), n)
				}
			}
		}
	}
}
