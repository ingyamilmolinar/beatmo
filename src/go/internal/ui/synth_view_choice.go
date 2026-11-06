package ui

// synth_view_choice.go — which view explains a dial ON THIS INSTRUMENT (spec
// docs/superpowers/specs/2026-10-25-synth-dial-relevance-and-cheap-graphs-design.md §3.2).
//
// A dial names the view that usually shows what it does (synth_dials.json
// "view"), but the same dial can change different things on different
// sounds: Room is a loudness tail on a kick and a colour on a tom. Every
// shown dial changes its instrument audibly (the relevance verdict), so its
// graph must show that change: when the named view does not move between
// the dial's two ends, the graph uses the view that moves most — measured on
// the two end renders the graph has anyway. Never an exception list.

import "math"

// viewMinMovePx is a visible move: the two ends' curves at least 3 px apart.
const viewMinMovePx = 3.0

// dialViewCandidates is the order the views are tried in (ties keep the first).
var dialViewCandidates = []synthView{viewLoudness, viewAttack, viewColour, viewColourTime, viewNoise, viewPitch, viewHarmonics, viewWave}

// viewMovePx is how far the HI end's curve stands from the LO end's (max over
// x, in pixels) when view v draws both on one w×h plot with shared axes.
func viewMovePx(v synthView, lo, hi *graphRender, w, h int) float64 {
	both := buildSynthViewPlot(v, w, h, synthViewInput{now: lo, hi: hi})
	var now, ghost [][]fpt
	for _, l := range both.lines {
		switch l.role {
		case roleNow:
			now = append(now, l.pts)
		case roleHi:
			ghost = append(ghost, l.pts)
		}
	}
	px := 0.0
	for _, g := range ghost {
		for _, p := range g {
			for _, n := range now {
				if y, ok := interpY(n, p.x); ok {
					px = math.Max(px, float64(absf(p.y-y)))
				}
			}
		}
	}
	// A line present at one end and absent at the other (no pitch at 0, a
	// pitch at 100) is a visible change too.
	if (len(now) == 0) != (len(ghost) == 0) {
		px = math.Max(px, viewMinMovePx)
	}
	return px
}

// chooseDialView returns the view that shows a dial's change: declared when
// it moves visibly between lo and hi, else the candidate that moves most.
// The ends must be analysed for every view (soundview.NeedAll) when declared
// does not move.
func chooseDialView(declared synthView, lo, hi *graphRender, w, h int) (synthView, float64) {
	best, bestPx := declared, viewMovePx(declared, lo, hi, w, h)
	if bestPx >= viewMinMovePx {
		return best, bestPx
	}
	for _, v := range dialViewCandidates {
		if v == declared {
			continue
		}
		if px := viewMovePx(v, lo, hi, w, h); px > bestPx {
			best, bestPx = v, px
		}
	}
	return best, bestPx
}
