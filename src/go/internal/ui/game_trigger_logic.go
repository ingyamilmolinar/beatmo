package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func (s *playbackSlate) incrementLogicTriggerCount(row int, id model.NodeID) int {
	g := s.g
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
func (s *playbackSlate) seqPrevTriggered(row, idx int, snap *seqPathSnapshot) (bool, bool) {
	g := s.g
	j := idx - 1
	prevIdx := -1
	prevID := model.InvalidNodeID
	for k := 0; k < 64; k++ {
		bi := seqBeatInfoAtRow(snap, row, j)
		if bi.NodeType.Fires() {
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
func (s *playbackSlate) seqShouldTriggerNode(row, idx int, info model.BeatInfo, n model.Node, counts map[model.NodeID]int, snap *seqPathSnapshot) bool {
	g := s.g
	if !info.NodeType.Fires() && info.NodeType != model.NodeTypeMute {
		return false
	}
	kind := n.Params.LogicKind
	switch kind {
	case model.LogicNone:
		return true
	case model.LogicEveryN:
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
	case model.LogicSkipEveryN:
		if n.Params.LogicN > 0 {
			if counts != nil {
				counts[info.NodeID]++
				if counts[info.NodeID]%n.Params.LogicN == 0 {
					return false
				}
			}
		}
	case model.LogicProbability:
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
	case model.LogicIfPrevSkipped:
		prevTrig, havePrev := g.seqPrevTriggered(row, idx, snap)
		if !havePrev || prevTrig {
			return false
		}
	case model.LogicIfPrevTriggered:
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

// nil (one voice) for every non-chord node.
func (s *playbackSlate) nodeLayersAt(info model.BeatInfo) []model.Layer {
	if info.NodeType != model.NodeTypeChord {
		return nil
	}
	n, ok := s.g.nodeSnapshot(info.NodeID)
	if !ok {
		return nil
	}
	return model.NodeLayers(n.Type, n.Params)
}

// No trigger counters or gating.
func (s *playbackSlate) evalNodeParamsOnly(row, idx int, info model.BeatInfo) (float64, float64, float64) {
	g := s.g
	if !info.NodeType.Fires() && info.NodeType != model.NodeTypeMute {
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
	}
	if gi := g.groupIndexSnapshot(); gi != nil && gi.HasRules(info.NodeID) {
		loopStart, loopLen := g.rowLoopSpan(row)
		pd, vm, dm := model.GroupEffect(gi, info.NodeID, idx, loopStart, loopLen)
		pitch += pd
		vol *= vm
		dur *= dm
		// Ruled nodes only, so ungrouped behaviour stays byte-identical.
		if pitch < -24 {
			pitch = -24
		} else if pitch > 24 {
			pitch = 24
		}
		if vol < 0 {
			vol = 0
		} else if vol > 4 {
			vol = 4
		}
		if dur < 0.1 {
			dur = 0.1
		} else if dur > 4 {
			dur = 4
		}
	}
	return vol, pitch, dur
}

func (s *playbackSlate) nodeTriggered(row, idx int, info model.BeatInfo) bool {
	g := s.g
	trig, _ := g.nodeTriggeredState(row, idx, info)
	return trig
}

func (s *playbackSlate) nodeTriggeredState(row, idx int, info model.BeatInfo) (bool, bool) {
	g := s.g
	if !info.NodeType.Fires() && info.NodeType != model.NodeTypeMute {
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

// Stable across runs and independent of the clock, so preview and playback agree on probability
// logic without shared RNG state.
func (s *playbackSlate) deterministicRoll(row, idx int, id model.NodeID) float64 {
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

func (g *Game) incrementLogicTriggerCount(row int, id model.NodeID) int {
	return g.playbackSlate.incrementLogicTriggerCount(row, id)
}

func (g *Game) seqPrevTriggered(row, idx int, snap *seqPathSnapshot) (bool, bool) {
	return g.playbackSlate.seqPrevTriggered(row, idx, snap)
}

func (g *Game) seqShouldTriggerNode(row, idx int, info model.BeatInfo, n model.Node, counts map[model.NodeID]int, snap *seqPathSnapshot) bool {
	return g.playbackSlate.seqShouldTriggerNode(row, idx, info, n, counts, snap)
}

func (g *Game) evalNodeParamsOnly(row, idx int, info model.BeatInfo) (float64, float64, float64) {
	return g.playbackSlate.evalNodeParamsOnly(row, idx, info)
}

func (g *Game) nodeTriggered(row, idx int, info model.BeatInfo) bool {
	return g.playbackSlate.nodeTriggered(row, idx, info)
}

func (g *Game) nodeTriggeredState(row, idx int, info model.BeatInfo) (bool, bool) {
	return g.playbackSlate.nodeTriggeredState(row, idx, info)
}

func (g *Game) deterministicRoll(row, idx int, id model.NodeID) float64 {
	return g.playbackSlate.deterministicRoll(row, idx, id)
}
