package ui

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// homeIdx is the node's first index in its row's flattened path; occurrences > 1 for a
// self-crossing circuit; inCircuit is false for a node on no path (no beat position).
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

// Prev and next fires honour logic gates: next from the Predictor, prev from the frozen Timeline.
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

// Valid while the playhead stays within [prevIdx, nextIdx).
type nodeTimingMemo struct {
	valid    bool
	id       model.NodeID
	pathSig  uint64
	playing  bool
	playhead int
	t        nodeTiming
}

// Each absolute index in a row maps 1:1 to one node, so timeline.Committed IS "did this node fire".
// Stopped: no prev, next from loop start.
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

	// A node fires at most once per period, so one period bounds every scan.
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

// The fraction is raw (den == MaxDiv), so a midpoint reads "4/8", never "1/2".
func (g *Game) beatSlotLabel(idx int) string {
	beat, num, den := g.grid.BeatSlot(idx)
	if num == 0 {
		return fmt.Sprintf("Beat %d", beat)
	}
	return fmt.Sprintf("Beat %d · %d/%d", beat, num, den)
}

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
