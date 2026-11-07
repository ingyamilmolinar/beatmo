package ui

import (
	"fmt"
	"math"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// Only while stopped, to avoid mid-flight desyncs.
func (g *Game) SetSubdivisions(n int) error {
	if err := g.validateSubdivisions(n); err != nil {
		return err
	}
	oldDiv := 1
	if g.grid != nil {
		oldDiv = g.grid.MaxDiv()
	}
	subs := []Subdivision{}
	// always include beat, halves, quarters when applicable
	base := []int{1, 2, 4, 8, 16, 32}
	for _, v := range base {
		if v <= n {
			subs = append(subs, Subdivision{Div: v, MinPx: 8, Style: LineStyle{Color: gridLineColorForDiv(v), Width: 1}})
		}
	}
	// Customize spacing similar to NewGrid (colors come from gridLineColorForDiv).
	for i := range subs {
		switch subs[i].Div {
		case 1:
			subs[i].MinPx = 0
		case 2:
			subs[i].MinPx = 32
		case 4:
			subs[i].MinPx = 14
		case 8:
			subs[i].MinPx = 10
		case 16:
			subs[i].MinPx = 10
		case 32:
			subs[i].MinPx = 8
		default:
			subs[i].MinPx = 8
		}
	}
	// Groove is relative to the subdivision, so scaling coordinates preserves timing and keeps edges
	// orthogonal.
	for id, node := range g.graph.Nodes {
		if node.Type == model.NodeTypeInvisible {
			continue
		}
		// Scale coordinates proportionally without groove adjustment
		iOld := float64(node.I) / float64(oldDiv)
		jOld := float64(node.J) / float64(oldDiv)

		// Round to nearest new grid step
		node.I = int(math.Round(iOld * float64(n)))
		node.J = int(math.Round(jOld * float64(n)))

		// Groove settings remain unchanged - they apply at playback time

		g.graph.Nodes[id] = node
		g.cacheNode(id)
		g.notifyPredictorNode(id)
	}
	// Scale absolute indices to preserve beat position under new grid.
	if oldDiv > 0 {
		scale := float64(n) / float64(oldDiv)
		newSteps := int(math.Round(float64(g.elapsedBeats) * scale))
		g.setPrimaryStep(newSteps)
		if len(g.nextBeatIdxs) > 0 {
			for i := range g.nextBeatIdxs {
				g.nextBeatIdxs[i] = int(math.Round(float64(g.nextBeatIdxs[i]) * scale))
			}
		}
		if len(g.nextBeatIdxs) > 0 {
			if len(g.seqNextIdxs) != len(g.nextBeatIdxs) {
				g.seqNextIdxs = make([]int, len(g.nextBeatIdxs))
			}
			copy(g.seqNextIdxs, g.nextBeatIdxs)
		}
		// Reset display beat baseline to current whole-beat position.
		beat := float64(g.elapsedBeats) / float64(n)
		g.state.SetLastDisplayBeat(beat)
		g.state.SetLastBeat(beat)
		g.state.SetLastProg(0)
	}
	g.grid.SetSubs(subs)
	// So hit tests and edits keep working at the new grid unit.
	unit := g.grid.Unit()
	for _, un := range g.nodes {
		if mn, ok := g.nodeSnapshot(un.ID); ok {
			un.I, un.J = mn.I, mn.J
			un.X, un.Y = float64(un.I)*unit, float64(un.J)*unit
		}
	}
	if g.drum != nil {
		g.drum.timelineUnitsPerBeat = g.grid.MaxDiv()
		if g.drum.subdivBtn() != nil {
			g.drum.subdivBtn().Text = fmt.Sprintf("\u00f7%d", g.grid.MaxDiv())
		}
	}
	g.edgesDirty = true
	// Old commits are at the old resolution, so their absolute indices no longer match.
	g.timeline = timeline.NewService()
	g.playedGraveyard = nil
	if len(g.frozenUpToByRow) != len(g.drum.Rows) {
		g.frozenUpToByRow = make([]int, len(g.drum.Rows))
	}
	for i := range g.frozenUpToByRow {
		g.frozenUpToByRow[i] = -1
	}
	g.clearParityState()
	g.updateBeatInfos()
	emitSubdivChange(n)
	return nil
}

// validateSubdivisions checks whether SetSubdivisions can be applied without mutating state.
func (g *Game) validateSubdivisions(n int) error {
	if n != 4 && n != 8 && n != 16 && n != 32 {
		return fmt.Errorf("invalid subdiv: %d", n)
	}
	if g.Playing() {
		return fmt.Errorf("cannot change subdiv while playing")
	}
	oldDiv := 1
	if g.grid != nil {
		oldDiv = g.grid.MaxDiv()
	}
	// Lowering is refused when an explicit node would fall off-grid; invisible intermediates do not count.
	if n < oldDiv {
		if oldDiv%n != 0 {
			return fmt.Errorf("cannot change subdiv: %d not a divisor of %d", n, oldDiv)
		}
		step := oldDiv / n
		for id, node := range g.graph.Nodes {
			if node.Type == model.NodeTypeInvisible {
				continue
			}
			if node.I%step != 0 || node.J%step != 0 {
				return fmt.Errorf("cannot lower subdiv to %d: node %d at (%d,%d) misaligned (need multiples of %d)", n, id, node.I, node.J, step)
			}
		}
	}
	return nil
}

// Removed global swing and per-instrument micro-delays; playback timing derives only from per-node groove settings.

// A fraction of a subdivision at the current BPM. Negative values are clamped at scheduling time
// so nothing lands in the past.
func (g *Game) nodeNudgeSecondsAt(row, idx int, info model.BeatInfo) float64 {
	bpm := g.AppliedBPM()
	if bpm <= 0 || g.grid == nil {
		return 0
	}
	secPerBeat := 60.0 / float64(bpm)
	sub := g.grid.MaxDiv()
	if sub <= 0 {
		return 0
	}
	secPerSub := secPerBeat / float64(sub)
	// A beat with a node plays that node's OWN groove and nothing else: borrowing a neighbour's offset
	// turned one swung offbeat into a shift of the whole phrase.
	if info.NodeID != model.InvalidNodeID {
		if n, ok := g.nodeSnapshot(info.NodeID); ok {
			if n.Params.GrooveKind != model.GrooveNone && n.Params.GroovePct != 0 {
				return n.Params.GrooveKind.Seconds(n.Params.GroovePct, secPerSub)
			}
			return 0
		}
	}
	// No resolvable node: borrow the most recent regular node's groove, searching at most two beats.
	maxBack := sub * 2
	for j := idx - 1; j >= 0 && j >= idx-maxBack; j-- {
		bi := g.beatInfoAtRow(row, j)
		if !bi.NodeType.Fires() || bi.NodeID == model.InvalidNodeID {
			continue
		}
		if n, ok := g.nodeSnapshot(bi.NodeID); ok {
			if n.Params.GrooveKind != model.GrooveNone && n.Params.GroovePct != 0 {
				return n.Params.GrooveKind.Seconds(n.Params.GroovePct, secPerSub)
			}
		}
	}
	return 0
}
