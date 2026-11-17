package ui

import (
	"fmt"
	"math"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Node sidebar value wheels: tapping a value pill (volume, duration, logic N,
// logic P, groove %) opens the shared value wheel over it. Each detent writes
// the node LIVE (setNodeParamsLive: the predictor hears the next trigger
// differently); closing commits ONCE (commitNodeParams: one log line, one undo
// step); Esc restores the opening params.

// nodeParamLimits is the ONE range table for node params, read by the sidebar
// ± steppers, the value wheels and the JS bridge (js_exports_graph_ui.go).
var nodeParamLimits = struct {
	VolumeMinDB, VolumeMaxDB float64 // wheel floor (below = Muted) / ceiling
	DurationMin, DurationMax float64
	LogicNMin, LogicNMax     int
}{
	VolumeMinDB: -40, VolumeMaxDB: 12,
	DurationMin: 0.25, DurationMax: 4,
	LogicNMin: 1, LogicNMax: 64,
}

// nodeVolumeMaxLinear is the +12 dB ceiling as a linear gain.
var nodeVolumeMaxLinear = math.Pow(10, nodeParamLimits.VolumeMaxDB/20)

// clampNodeVolume caps a linear node volume at the +12 dB ceiling.
func clampNodeVolume(v float64) float64 {
	if v > nodeVolumeMaxLinear {
		return nodeVolumeMaxLinear
	}
	return v
}

// clampNodeDuration keeps a duration multiplier inside the limits; 0 (the
// unset default) reads as 1.
func clampNodeDuration(d float64) float64 {
	if d == 0 {
		d = 1
	}
	return math.Max(nodeParamLimits.DurationMin, math.Min(nodeParamLimits.DurationMax, d))
}

// clampLogicN keeps a logic count inside the limits.
func clampLogicN(n int) int {
	if n < nodeParamLimits.LogicNMin {
		return nodeParamLimits.LogicNMin
	}
	if n > nodeParamLimits.LogicNMax {
		return nodeParamLimits.LogicNMax
	}
	return n
}

// nodeVolumeMutedDB is the volume wheel's bottom row: Muted (gain 0).
var nodeVolumeMutedDB = nodeParamLimits.VolumeMinDB - 1

// setNodeParamsLive writes p to the node and tells the predictor, WITHOUT
// announcing it (the wheel's per-detent write). logic re-bases the predictor
// for logic-gate params, as the steppers do.
func (g *Game) setNodeParamsLive(id model.NodeID, p model.NodeParams, logic bool) {
	g.graph.SetNodeParams(id, p)
	g.notifyPredictorNode(id)
	if logic && g.engine != nil && g.engine.Predictor != nil {
		g.engine.Predictor.RebaseAt(g.elapsedBeats)
	}
}

// commitNodeParams announces a node's current params once (log line + undo
// step) — the gesture commit for a value wheel.
func (g *Game) commitNodeParams(id model.NodeID) {
	if mn, ok := g.graph.GetNodeByID(id); ok {
		emitNodeParamsChanged(id, mn.Params)
	}
}

// openNodeValueWheel opens the value wheel for one of the node's value pills
// (volume, duration, logic N / P, groove %). The node id is captured here, so a wheel
// left open while the sidebar retargets still commits to its own node.
func (g *Game) openNodeValueWheel(kind sbField) {
	sb := g.sidebar
	if sb == nil || sb.node == nil || g.drum == nil {
		return
	}
	id := sb.node.ID
	mn, ok := g.graph.GetNodeByID(id)
	if !ok {
		return
	}
	anchor := sb.rects[sbValue(kind)]
	start := mn.Params
	var m *NumberModel
	var title string
	var apply func(p *model.NodeParams, v float64)
	logic := false
	switch kind {
	case fieldVol:
		// The bottom row is Muted. Normally that is one step below -40 dB, but a
		// node the − stepper took quieter than that keeps its real level on the
		// wheel (the floor moves down to just below it).
		mutedDB := nodeVolumeMutedDB
		v := mutedDB
		if start.Volume > 0 {
			v = 20 * math.Log10(start.Volume)
			if v-1 < mutedDB {
				mutedDB = math.Floor(v) - 1
			}
		}
		isMuted := func(v float64) bool { return v <= mutedDB+0.25 }
		m = &NumberModel{Min: mutedDB, Max: math.Max(nodeParamLimits.VolumeMaxDB, v), Ladder: []float64{0.5, 1, 3, 6}, Rung: 1,
			Format: func(v, step float64) string {
				if isMuted(v) {
					return formatNodeVolumeDb(0)
				}
				if math.Abs(v) < 1e-9 {
					return "0 dB"
				}
				if step < 1 {
					return fmt.Sprintf("%+.1f dB", v)
				}
				return fmt.Sprintf("%+d dB", int(math.Round(v)))
			},
			LadderFormat: func(s float64) string { return trimFloat(s) + " dB" }}
		m.Value = m.clamp(v)
		title = i18n.T(i18n.KeyNodeSecVolume)
		apply = func(p *model.NodeParams, v float64) {
			if isMuted(v) {
				p.Volume = 0
				return
			}
			p.Volume = math.Pow(10, v/20)
			if v <= nodeParamLimits.VolumeMaxDB {
				p.Volume = clampNodeVolume(p.Volume)
			}
		}
	case fieldDur:
		m = &NumberModel{Min: nodeParamLimits.DurationMin, Max: nodeParamLimits.DurationMax, Ladder: []float64{0.05, 0.1, 0.25, 1}, Rung: 1,
			Format:       func(v, _ float64) string { return fmt.Sprintf("%.2fx", v) },
			LadderFormat: func(s float64) string { return trimFloat(s) + "x" }}
		m.Value = clampNodeDuration(start.Duration)
		title = i18n.T(i18n.KeyNodeSecDuration)
		apply = func(p *model.NodeParams, v float64) { p.Duration = v }
	case fieldLogicN:
		m = &NumberModel{Min: float64(nodeParamLimits.LogicNMin), Max: float64(nodeParamLimits.LogicNMax), Ladder: []float64{1, 4, 16},
			Format: func(v, _ float64) string { return fmt.Sprintf("%d", int(v)) }}
		m.Value = float64(clampLogicN(start.LogicN))
		title = i18n.T(i18n.KeyNodeSecLogic)
		logic = true
		apply = func(p *model.NodeParams, v float64) { p.LogicN = int(v) }
	case fieldLogicP:
		m = &NumberModel{Min: 0, Max: 100, Ladder: []float64{1, 5, 10, 25}, Rung: 2,
			Format:       func(v, _ float64) string { return fmt.Sprintf("%.0f%%", v) },
			LadderFormat: func(s float64) string { return trimFloat(s) + "%" }}
		m.Value = m.clamp(start.LogicP * 100)
		title = i18n.T(i18n.KeyLogicProbability)
		logic = true
		apply = func(p *model.NodeParams, v float64) { p.LogicP = v / 100 }
	case fieldGroovePct:
		m = &NumberModel{Min: 0, Max: 100, Ladder: []float64{1, 5, 25}, Rung: 1,
			Format:       func(v, _ float64) string { return fmt.Sprintf("%.0f%%", v) },
			LadderFormat: func(s float64) string { return trimFloat(s) + "%" }}
		m.Value = m.clamp(start.GroovePct * 100)
		title = i18n.T(i18n.KeyNodeSecGroove)
		apply = func(p *model.NodeParams, v float64) { p.GroovePct = v / 100 }
	default:
		return
	}
	dv := g.drum
	dv.openValueWheelIn(WheelBinding{
		Model: m,
		Title: func() string { return title },
		OnChange: func() {
			cur, ok := g.graph.GetNodeByID(id)
			if !ok {
				return
			}
			p := cur.Params
			apply(&p, m.Value)
			g.setNodeParamsLive(id, p, logic)
		},
		// Esc puts the node back EXACTLY as it was (the model converts and
		// clamps, so re-deriving from it would drift or mute a quiet node).
		OnCancel: func() { g.setNodeParamsLive(id, start, logic) },
		OnCommit: func() { g.commitNodeParams(id) },
	}, anchor, g.windowRect())
}
