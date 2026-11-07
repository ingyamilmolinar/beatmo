package ui

// The same dial changes different things on different sounds (Room: a tail on a kick, a colour on a tom), so
// when its named view does not move between its ends the graph uses the view that moves most. No exception list.

import "math"

// viewMinMovePx is a visible move: the two ends' curves at least 3 px apart.
const viewMinMovePx = 3.0

// dialViewCandidates is the order the views are tried in (ties keep the first).
var dialViewCandidates = []synthView{viewLoudness, viewAttack, viewColour, viewColourTime, viewNoise, viewPitch, viewHarmonics, viewWave}

// Max over x, in pixels, with shared axes.
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
	// A line present at one end and absent at the other is a visible change too.
	if (len(now) == 0) != (len(ghost) == 0) {
		px = math.Max(px, viewMinMovePx)
	}
	return px
}

// When declared does not move, the ends must be analysed for every view.
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
