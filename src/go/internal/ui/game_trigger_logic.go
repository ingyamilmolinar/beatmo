package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Backward-compatible wrapper used by tests that expect (id, vol).
func (g *Game) queueSound(id string, vol float64) { g.queueSoundParams(id, vol, 0, 1) }

func (g *Game) incrementTriggerCount(row int, id model.NodeID) int {
	if _, ok := g.nodeTriggerCountsByRow[row]; !ok {
		g.nodeTriggerCountsByRow[row] = make(map[model.NodeID]int)
	}
	g.nodeTriggerCountsByRow[row][id] = g.nodeTriggerCountsByRow[row][id] + 1
	if os.Getenv("PREVIEW_DEBUG") == "1" && runningUnderGoTest() {
		fmt.Printf("INC row=%d id=%d -> %d\n", row, id, g.nodeTriggerCountsByRow[row][id])
	}
	return g.nodeTriggerCountsByRow[row][id]
}

func (g *Game) incrementLogicTriggerCount(row int, id model.NodeID) int {
	if g.nodeLogicTriggerCountsByRow == nil {
		g.nodeLogicTriggerCountsByRow = make(map[int]map[model.NodeID]int)
	}
	if _, ok := g.nodeLogicTriggerCountsByRow[row]; !ok {
		g.nodeLogicTriggerCountsByRow[row] = make(map[model.NodeID]int)
	}
	g.nodeLogicTriggerCountsByRow[row][id] = g.nodeLogicTriggerCountsByRow[row][id] + 1
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
			} else {
				if counts != nil {
					counts[info.NodeID] = counts[info.NodeID] + 1
					if counts[info.NodeID]%n.Params.LogicN != 0 {
						return false
					}
				}
			}
		}
	case "skip_every_n":
		if n.Params.LogicN > 0 {
			if counts != nil {
				counts[info.NodeID] = counts[info.NodeID] + 1
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

func (g *Game) shouldTriggerNode(row, idx int, info model.BeatInfo, n model.Node) bool {
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
				cycleLen := len(g.beatInfosByRow[row])
				if cycleLen <= 0 {
					cycleLen = 1
				}
				cycle := idx / cycleLen
				if (cycle+1)%n.Params.LogicN != 0 {
					return false
				}
			} else {
				if g.incrementTriggerCount(row, info.NodeID)%n.Params.LogicN != 0 {
					return false
				}
			}
		}
	case "skip_every_n":
		if n.Params.LogicN > 0 {
			if g.incrementTriggerCount(row, info.NodeID)%n.Params.LogicN == 0 {
				return false
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
		prevTrig, havePrev := g.prevTriggered(row, idx)
		if !havePrev || prevTrig {
			return false
		}
	case "trigger_if_prev_triggered":
		prevTrig, havePrev := g.prevTriggered(row, idx)
		if !havePrev || !prevTrig {
			return false
		}
	}
	return true
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

// hasPrevRegular reports whether there exists a regular node before idx on the row.
func (g *Game) hasPrevRegular(row, idx int) bool {
	j := idx - 1
	for k := 0; k < 64; k++ {
		bi := g.beatInfoAtRow(row, j)
		if bi.NodeType == model.NodeTypeRegular {
			return true
		}
		j--
	}
	return false
}

// isSeamSuppressed returns true when idx falls on a loop seam duplicate that
// should be visually and audibly suppressed.
func (g *Game) isSeamSuppressed(row, idx int) bool {
	if row < 0 || row >= len(g.isLoopByRow) {
		return false
	}
	if !g.isLoopByRow[row] {
		return false
	}
	start := 0
	if row < len(g.loopStartByRow) {
		start = g.loopStartByRow[row]
	}
	seg := 0
	if row < len(g.loopLenByRow) {
		seg = g.loopLenByRow[row]
	}
	if seg <= 0 {
		return false
	}
	// Suppress the first step inside each loop cycle to avoid a visual/audio
	// double-hit across the seam (the returning step connecting to the loop start).
	if idx >= start+1 {
		if (idx-(start+1))%seg == 0 {
			info := g.beatInfoAtRow(row, idx)
			if info.NodeType == model.NodeTypeInvisible {
				return true
			}
		}
	}
	return false
}

// prevTriggeredByPrediction determines if the nearest previous regular node
// before idx fired audibly, using precomputed predictions. This avoids timing
// dependencies on UI thread updates.
// prevTriggered determines whether the nearest previous regular node fired.
// It first consults the lastTriggeredByRow override (used by unit tests and UI
// thread), and falls back to prediction-based lookup when no explicit state is
// available. The second return value reports whether a previous regular exists.
func (g *Game) prevTriggered(row, idx int) (bool, bool) {
	// Find prev regular index and ID.
	j := idx - 1
	prevIdx := -1
	var prevID model.NodeID = model.InvalidNodeID
	for k := 0; k < 64; k++ {
		bi := g.beatInfoAtRow(row, j)
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
	// Prefer explicit lastTriggered state when present (tests/UI thread).
	if v, ok := g.lastTriggered(row, prevID); ok {
		return v, true
	}
	// Fallback to engine prediction-backed audible state.
	need := prevIdx + 1
	if g.engine == nil || g.engine.Predictor == nil {
		return false, true
	}
	g.engine.Predictor.Ensure(need)
	return g.engine.Predictor.AudibleAt(row, prevIdx), true
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
