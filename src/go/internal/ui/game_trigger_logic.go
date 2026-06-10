package ui

import (
	"strings"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func (g *Game) incrementLogicTriggerCount(row int, id model.NodeID) int {
	if g.nodeLogicTriggerCountsByRow == nil {
		g.nodeLogicTriggerCountsByRow = make(map[int]map[model.NodeID]int)
	}
	if _, ok := g.nodeLogicTriggerCountsByRow[row]; !ok {
		g.nodeLogicTriggerCountsByRow[row] = make(map[model.NodeID]int)
	}
	g.nodeLogicTriggerCountsByRow[row][id]++
	return g.nodeLogicTriggerCountsByRow[row][id]
}

// seqPrevTriggered mirrors prevTriggered but uses the sequencer path snapshot.
func (g *Game) seqPrevTriggered(row, idx int, snap *seqPathSnapshot) (bool, bool) {
	j := idx - 1
	prevIdx := -1
	prevID := model.InvalidNodeID
	for k := 0; k < 64; k++ {
		bi := seqBeatInfoAtRow(snap, row, j)
		if bi.NodeType == model.NodeTypeRegular {
			prevIdx = j
			prevID = bi.NodeID
			break
		}
		j--
	}
	if prevIdx < 0 {
		return false, false
	}
	if v, ok := g.lastTriggered(row, prevID); ok {
		return v, true
	}
	need := prevIdx + 1
	if g.engine == nil || g.engine.Predictor == nil {
		return false, true
	}
	g.engine.Predictor.Ensure(need)
	return g.engine.Predictor.AudibleAt(row, prevIdx), true
}

// seqShouldTriggerNode evaluates built-in logic rules using the sequencer path snapshot.
func (g *Game) seqShouldTriggerNode(row, idx int, info model.BeatInfo, n model.Node, counts map[model.NodeID]int, snap *seqPathSnapshot) bool {
	if info.NodeType != model.NodeTypeRegular && info.NodeType != model.NodeTypeMute {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(n.Params.LogicKind))
	switch kind {
	case "", "none":
		return true
	case "every_n_triggers":
		if n.Params.LogicN > 0 {
			if info.NodeType == model.NodeTypeMute {
				cycleLen := 0
				if snap != nil && row >= 0 && row < len(snap.beatInfosByRow) {
					cycleLen = len(snap.beatInfosByRow[row])
				}
				if cycleLen <= 0 {
					cycleLen = 1
				}
				cycle := idx / cycleLen
				if (cycle+1)%n.Params.LogicN != 0 {
					return false
				}
			} else if counts != nil {
				counts[info.NodeID]++
				if counts[info.NodeID]%n.Params.LogicN != 0 {
					return false
				}
			}
		}
	case "skip_every_n":
		if n.Params.LogicN > 0 {
			if counts != nil {
				counts[info.NodeID]++
				if counts[info.NodeID]%n.Params.LogicN == 0 {
					return false
				}
			}
		}
	case "probability":
		p := n.Params.LogicP
		if p <= 0 {
			return false
		}
		if p < 1 {
			r := g.deterministicRoll(row, idx, info.NodeID)
			if r > p {
				return false
			}
		}
	case "trigger_if_prev_skipped":
		prevTrig, havePrev := g.seqPrevTriggered(row, idx, snap)
		if !havePrev || prevTrig {
			return false
		}
	case "trigger_if_prev_triggered":
		prevTrig, havePrev := g.seqPrevTriggered(row, idx, snap)
		if !havePrev || !prevTrig {
			return false
		}
	}
	return true
}

func applyNodeDecision(vol, pitch, dur float64, dec model.NodeDecision) (float64, float64, float64) {
	if dec.VolumeMul != 0 {
		vol *= dec.VolumeMul
	}
	if dec.PitchDelta != 0 {
		pitch += dec.PitchDelta
	}
	if dec.DurationMul != 0 {
		dur *= dec.DurationMul
	}
	return vol, pitch, dur
}

// evalNodeParamsOnly computes effective volume/pitch/duration from node
// parameters without touching trigger counters or applying gating logic.
func (g *Game) evalNodeParamsOnly(row, idx int, info model.BeatInfo) (float64, float64, float64) {
	if info.NodeType != model.NodeTypeRegular && info.NodeType != model.NodeTypeMute {
		return 0, 0, 1
	}
	vol := 1.0
	if row >= 0 && row < len(g.drum.Rows) {
		vol = g.drum.Rows[row].Volume
	}
	pitch := 0.0
	dur := 1.0
	if n, ok := g.nodeSnapshot(info.NodeID); ok {
		vol *= n.Params.Volume
		pitch += n.Params.Pitch
		dur *= n.Params.Duration
		// Do not apply user logic or trigger-based rules here; gating and
		// counters are handled elsewhere by predictions/sequencer.
	}
	return vol, pitch, dur
}

func (g *Game) nodeTriggered(row, idx int, info model.BeatInfo) bool {
	trig, _ := g.nodeTriggeredState(row, idx, info)
	return trig
}

func (g *Game) nodeTriggeredState(row, idx int, info model.BeatInfo) (bool, bool) {
	if info.NodeType != model.NodeTypeRegular && info.NodeType != model.NodeTypeMute {
		return false, true
	}
	if _, ok := g.graph.GetNodeByID(info.NodeID); !ok {
		return false, false
	}
	if row >= 0 {
		if v, ok := g.lastTriggered(row, info.NodeID); ok {
			return v, true
		}
	}
	need := idx + 1
	if g.engine == nil || g.engine.Predictor == nil {
		return false, true
	}
	g.engine.Predictor.Ensure(need)
	return g.engine.Predictor.TriggeredAt(row, idx), true
}

// deterministicRoll returns a deterministic pseudo-random value in [0,1) for
// the given row/index/node triple, stable across runs and independent of wall
// clock, so preview predictions and audio playback can agree for probability
// logic without shared mutable RNG state.
func (g *Game) deterministicRoll(row, idx int, id model.NodeID) float64 {
	// 64-bit SplitMix-derived hash
	x := uint64(uint32(row))<<32 ^ uint64(uint32(idx)) ^ (uint64(uint32(id)) << 16) ^ 0x9E3779B97F4A7C15
	x += 0x9E3779B97F4A7C15
	z := x
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	z ^= (z >> 31)
	// Map lower 53 bits to [0,1)
	return float64(z&((1<<53)-1)) / float64(1<<53)
}
