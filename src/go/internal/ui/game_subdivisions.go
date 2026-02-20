package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// SetSubdivisions updates the grid subdivisions-per-beat (allowed: 4,8,16,32).
// It is only applied when playback is stopped to avoid mid-flight desyncs.
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
			subs = append(subs, Subdivision{Div: v, MinPx: 8, Style: LineStyle{Color: colGridLine, Width: 1}})
		}
	}
	// Customize styles similar to NewGrid
	for i := range subs {
		switch subs[i].Div {
		case 1:
			subs[i].Style.Color = colGridLine
			subs[i].MinPx = 0
		case 2:
			subs[i].Style.Color = colGridHalf
			subs[i].MinPx = 32
		case 4:
			subs[i].Style.Color = colGridQuarter
			subs[i].MinPx = 14
		case 8:
			subs[i].Style.Color = colGridEighth
			subs[i].MinPx = 10
		case 16:
			subs[i].Style.Color = colGridSixteenth
			subs[i].MinPx = 10
		case 32:
			subs[i].Style.Color = colGridThirtySecond
			subs[i].MinPx = 8
		default:
			subs[i].MinPx = 8
		}
	}
	// oldDiv already captured above
	// Scale node coordinates proportionally without incorporating groove timing.
	// Groove settings remain unchanged - they apply at playback time relative to
	// the subdivision, so timing is preserved naturally. This approach ensures
	// edges between connected nodes remain orthogonal after the transformation.
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
	// Sync UI node coordinates (I,J,X,Y) to the remapped graph nodes so hit
	// tests and add/remove operations continue to work after subdivision
	// changes. Use the new grid unit for world coordinates.
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
	// Subdivision changes invalidate prior timeline commits recorded at the
	// previous resolution. Clear timeline/parity state so playback/history
	// can rebuild under the new grid without mismatched absolute indices.
	g.timeline = timeline.NewService()
	if len(g.frozenUpToByRow) != len(g.drum.Rows) {
		g.frozenUpToByRow = make([]int, len(g.drum.Rows))
	}
	for i := range g.frozenUpToByRow {
		g.frozenUpToByRow[i] = -1
	}
	g.clearParityState()
	g.updateBeatInfos()
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
	// Disallow lowering resolution when explicit nodes would fall off-grid.
	// A node at coordinates (I,J) in oldDiv units is representable at the new
	// grid iff I and J are multiples of (oldDiv/newDiv). Ignore invisible
	// intermediates; only explicit audible/silent nodes gate this change.
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

// nodeNudgeSeconds reads the selected node's NudgePct and converts it into seconds
// as a fraction of an 8th note at the current BPM. Negative values are clamped
// to zero at scheduling time to avoid past timestamps; UI may still store them.
// nodeNudgeSecondsAt returns the groove offset in seconds for a beat event at
// (row, idx). If the current node has no groove configured, it falls back to
// the most recent regular node before idx (within a reasonable search window)
// so per-node groove can apply to the immediately following trigger.
func (g *Game) nodeNudgeSecondsAt(row, idx int, info model.BeatInfo) float64 {
	// identify the BeatInfo at this row/idx for the primary row context if necessary
	// We only need BPM and grid to compute seconds per 8th; NodeParams are applied
	// in scheduleSound via the currently-resolved node.
	bpm := g.AppliedBPM()
	if bpm <= 0 || g.grid == nil {
		return 0
	}
	// helper to compute seconds per subdivision and scale from groove pct
	// One subdivision duration
	secPerBeat := 60.0 / float64(bpm)
	sub := g.grid.MaxDiv()
	if sub <= 0 {
		return 0
	}
	secPerSub := secPerBeat / float64(sub)
	compute := func(kind string, pct float64) float64 {
		if pct < 0 {
			pct = 0
		}
		if pct > 1 {
			pct = 1
		}
		d := pct * secPerSub
		switch strings.ToLower(kind) {
		case "delay":
			return d
		case "rush":
			return -d
		default:
			return 0
		}
	}
	// First, try the current node.
	if info.NodeID != model.InvalidNodeID {
		if n, ok := g.nodeSnapshot(info.NodeID); ok {
			if n.Params.GrooveKind != "" && n.Params.GroovePct != 0 {
				return compute(n.Params.GrooveKind, n.Params.GroovePct)
			}
		}
		// For regular nodes with no groove, we still consider the most
		// recent regular node’s groove as a micro‑timing offset to apply to
		// the immediately following trigger. This preserves the intended
		// semantics where a node’s groove affects the next audible event.
	}
	// Otherwise, search backwards for the most recent regular node and apply its groove.
	// Limit search to two beats worth of subdivisions to bound cost.
	maxBack := sub * 2
	for j := idx - 1; j >= 0 && j >= idx-maxBack; j-- {
		bi := g.beatInfoAtRow(row, j)
		if bi.NodeType != model.NodeTypeRegular || bi.NodeID == model.InvalidNodeID {
			continue
		}
		if n, ok := g.nodeSnapshot(bi.NodeID); ok {
			if n.Params.GrooveKind != "" && n.Params.GroovePct != 0 {
				return compute(n.Params.GrooveKind, n.Params.GroovePct)
			}
		}
	}
	return 0
}
