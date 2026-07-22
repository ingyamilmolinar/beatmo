package ui

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// nodeCircuitPosition resolves a node's static position within its traversal
// path. row is the node's row (from g.nodeRows); homeIdx is the first index at
// which the node appears in that row's flattened path (beatInfosByRow), which is
// the absolute subdivision count from the circuit's start. occurrences counts how
// many times the node appears in one path slice (a self-crossing circuit visits a
// node more than once). inCircuit is false when the node participates in no path
// (a disconnected / open dead-end node), in which case it has no beat position.
func (g *Game) nodeCircuitPosition(id model.NodeID) (homeIdx, occurrences, row int, inCircuit bool) {
	r, ok := g.nodeRows[id]
	if !ok || r < 0 || r >= len(g.beatInfosByRow) {
		return 0, 0, -1, false
	}
	infos := g.beatInfosByRow[r]
	homeIdx = -1
	for i := range infos {
		if infos[i].NodeID == id {
			if homeIdx < 0 {
				homeIdx = i
			}
			occurrences++
		}
	}
	if homeIdx < 0 {
		return 0, 0, r, false
	}
	return homeIdx, occurrences, r, true
}

// nodeTiming is the derived timing readout for one node: its static home position
// in its circuit plus (during playback) its previous and next ACTUAL fire relative
// to the playhead. "Actual" means honoring logic gates (probability, skip-every-N):
// next is read from the Predictor, prev from the frozen Timeline.
type nodeTiming struct {
	inCircuit   bool
	homeIdx     int
	occurrences int
	row         int

	prevIdx int
	prevOK  bool
	nextIdx int
	nextOK  bool
}

// nodeTimingMemo caches one nodeTiming keyed by node id + row path signature +
// play state. It stays valid while the playhead sits within [prevIdx, nextIdx);
// crossing nextIdx (or seeking back past prevIdx) forces a recompute.
type nodeTimingMemo struct {
	valid    bool
	id       model.NodeID
	pathSig  uint64
	playing  bool
	playhead int
	t        nodeTiming
}

// nodeLiveTiming returns the node's home position and its live last/next fire.
// Next fire is sampled from the Predictor (future, honors logic gates); prev fire
// from the Timeline (frozen past). Because each absolute index in a row maps 1:1
// to one node, timeline.Committed's bool IS "did this node fire at idx". When
// stopped, prev is omitted and next is computed from loop start (position 0).
func (g *Game) nodeLiveTiming(id model.NodeID) nodeTiming {
	home, occ, row, inCircuit := g.nodeCircuitPosition(id)
	if !inCircuit {
		g.nodeTimingCache = nodeTimingMemo{}
		return nodeTiming{inCircuit: false}
	}

	playing := g.Playing()
	playhead := g.elapsedBeats
	var sig uint64
	if row >= 0 && row < len(g.pathSigByRow) {
		sig = g.pathSigByRow[row]
	}

	// Memo hit: same node/path/play-state, playhead still within the cached span.
	if m := &g.nodeTimingCache; m.valid && m.id == id && m.pathSig == sig && m.playing == playing {
		fresh := true
		if playing {
			if m.t.nextOK && playhead >= m.t.nextIdx {
				fresh = false // crossed the next fire
			}
			if m.t.prevOK && playhead < m.t.prevIdx {
				fresh = false // seeked back past the prev fire
			}
			if !m.t.prevOK && playhead != m.playhead {
				fresh = false // a prev may now exist
			}
		}
		if fresh {
			return m.t
		}
	}

	t := nodeTiming{inCircuit: true, homeIdx: home, occurrences: occ, row: row}

	// One full loop period bounds every scan: a node fires at most once per period,
	// so one period is enough to find the nearest fire or conclude there is none.
	period := len(g.beatInfosByRow[row])
	if g.isLoopByRow[row] {
		period = len(g.beatInfosByRow[row]) - g.loopStartByRow[row]
	}
	if period < 1 {
		period = 1
	}

	start := playhead
	if !playing {
		start = 0 // stopped: next-from-0
	}

	// Next actual fire: first candidate idx >= start the Predictor triggers.
	if g.engine != nil && g.engine.Predictor != nil {
		horizon := g.engine.Predictor.Horizon()
		for idx := start; idx <= start+period; idx++ {
			if horizon > 0 && idx >= horizon {
				break
			}
			if g.beatInfoAtRow(row, idx).NodeID == id && g.engine.Predictor.TriggeredAt(row, idx) {
				t.nextIdx = idx
				t.nextOK = true
				break
			}
		}
	}

	// Prev actual fire (playing only): largest committed idx < playhead.
	if playing && g.timeline != nil {
		lo := playhead - period
		if lo < 0 {
			lo = 0
		}
		for idx := playhead - 1; idx >= lo; idx-- {
			if g.beatInfoAtRow(row, idx).NodeID != id {
				continue
			}
			if fired, _, ok := g.timeline.Committed(row, idx); ok && fired {
				t.prevIdx = idx
				t.prevOK = true
				break
			}
		}
	}

	g.nodeTimingCache = nodeTimingMemo{
		valid: true, id: id, pathSig: sig, playing: playing, playhead: playhead, t: t,
	}
	return t
}

// beatSlotLabel renders a subdivision index as "Beat N · k/D" — k of D
// subdivisions into beat N — collapsing to just "Beat N" when the index lands
// exactly on the beat (k == 0). The fraction is raw (den == MaxDiv), so the
// midpoint between two beat-aligned nodes reads "4/8", not "1/2" or "5/8".
func (g *Game) beatSlotLabel(idx int) string {
	beat, num, den := g.grid.BeatSlot(idx)
	if num == 0 {
		return fmt.Sprintf("Beat %d", beat)
	}
	return fmt.Sprintf("Beat %d · %d/%d", beat, num, den)
}

// formatNodeCoordHome renders the always-visible position line:
//
//	"(5, 2) · Beat 3 · 4/8"          — in a circuit, mid-beat
//	"(5, 2) · Beat 3"                — in a circuit, exactly on the beat
//	"(5, 2) · Beat 3 · 4/8 (+1)"     — appears more than once (self-crossing)
//	"(9, 9) · off-circuit"           — participates in no path
func (g *Game) formatNodeCoordHome(i, j int, t nodeTiming) string {
	if !t.inCircuit {
		return fmt.Sprintf("(%d, %d) · off-circuit", i, j)
	}
	s := fmt.Sprintf("(%d, %d) · %s", i, j, g.beatSlotLabel(t.homeIdx))
	if t.occurrences > 1 {
		s += fmt.Sprintf(" (+%d)", t.occurrences-1)
	}
	return s
}

// formatNodeFireLine renders the live last/next line. Empty when off-circuit.
// Stopped: "next  Beat 3 · 4/8". Playing: "last  Beat 2 · 4/8    next  Beat 6 · 4/8".
// A missing side reads as an em dash.
func (g *Game) formatNodeFireLine(t nodeTiming, playing bool) string {
	if !t.inCircuit {
		return ""
	}
	part := func(prefix string, idx int, ok bool) string {
		if !ok {
			return prefix + "  —"
		}
		return prefix + "  " + g.beatSlotLabel(idx)
	}
	if !playing {
		return part("next", t.nextIdx, t.nextOK)
	}
	return part("last", t.prevIdx, t.prevOK) + "    " + part("next", t.nextIdx, t.nextOK)
}
