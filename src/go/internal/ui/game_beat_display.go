package ui

import (
	"math"
	"time"

	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

func (g *Game) currentBeat() float64 {
	div := g.grid.MaxDiv()
	if div <= 0 {
		div = 1
	}
	base := float64(g.elapsedBeats) / float64(div)
	if !g.Playing() {
		return base
	}
	prog := g.engineProgress()
	lastProg := g.state.LastProg()
	frac := prog
	if lastProg > 0 && prog+0.25 < lastProg {
		frac = 1 + prog
	} else if prog < lastProg {
		frac = lastProg
	}
	beat := base + frac
	q := math.Round(beat*float64(div)) / float64(div)
	lastBeat := g.state.LastBeat()
	if q < lastBeat {
		q = lastBeat
	}
	g.state.SetLastProg(prog)
	g.state.SetLastBeat(q)
	return q
}

// displayBeat returns a smooth beat position suitable for UI timers. It
// combines the completed subdivision steps with the scheduler's fractional
// progress and clamps minor regressions for jitter-free display. It never
// lags behind the last completed subdivision within the current beat.
func (g *Game) displayBeat() float64 {
	div := g.grid.MaxDiv()
	if div <= 0 {
		div = 1
	}
	lastDisplay := g.state.LastDisplayBeat()
	if !g.Playing() {
		if lastDisplay > 0 {
			return lastDisplay
		}
		return float64(g.elapsedBeats) / float64(div)
	}
	if g.state.PlayStart().IsZero() {
		baseWhole := g.elapsedBeats / div
		base := float64(baseWhole)
		if p := g.pulseForRow(0); p != nil {
			seg := p.segBeats
			if seg <= 0 {
				dist := hypot(p.x2-p.x1, p.y2-p.y1)
				seg = dist / g.grid.Step
				if seg <= 0 {
					seg = 1.0 / float64(div)
				}
			}
			v := base + p.t*seg
			bpm := g.AppliedBPM()
			if bpm <= 0 {
				bpm = g.bpm
			}
			if bpm > 0 {
				minDelta := float64(bpm) / 60000.0
				if v < lastDisplay+minDelta {
					maxV := base + seg
					candidate := lastDisplay + minDelta
					if candidate < maxV {
						v = candidate
					} else {
						v = maxV
					}
				}
			}
			if v < lastDisplay {
				v = lastDisplay
			}
			g.state.SetLastDisplayBeat(v)
			return v
		}
		prog := g.engineProgress()
		lastProg := g.state.LastProg()
		frac := prog
		if lastProg > 0 && prog+0.25 < lastProg {
			frac = 1 + prog
		} else if prog < lastProg {
			frac = lastProg
		}
		v := base + frac
		if v < lastDisplay {
			v = lastDisplay
		}
		g.state.SetLastProg(prog)
		g.state.SetLastDisplayBeat(v)
		return v
	}

	bpm := g.AppliedBPM()
	if bpm <= 0 {
		bpm = g.bpm
	}
	if bpm <= 0 {
		return lastDisplay
	}
	audioStart := g.state.AudioStart()
	var dtSec float64
	if n := audio.Now(); n > 0 && audioStart > 0 {
		dtSec = n - audioStart
	} else if !g.state.PlayStart().IsZero() {
		dtSec = time.Since(g.state.PlayStart()).Seconds()
	} else {
		return lastDisplay
	}
	v := g.state.BeatBase() + dtSec*float64(bpm)/60.0
	if v < lastDisplay {
		v = lastDisplay
	}
	g.state.SetLastDisplayBeat(v)
	return v
}

func (g *Game) rootNode() *uiNode {
	if g.start != nil {
		return g.start
	}
	var root *uiNode
	for _, n := range g.nodes {
		if n.J != 0 {
			continue
		}
		inbound := false
		for _, e := range g.edges {
			if e.B == n {
				inbound = true
				break
			}
		}
		if !inbound {
			if root == nil || n.I < root.I {
				root = n
			}
		}
	}
	return root
}

func (g *Game) pulseForRow(row int) *pulse {
	for _, p := range g.activePulses {
		if p.row == row {
			return p
		}
	}
	return nil
}

const highlightMuteFlag int64 = 1 << 62
