package engine

import (
	"strings"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func muteHoldStepsFromNode(n model.Node) int {
	// Mute nodes no longer carry a duration-based gate.
	return 0
}

func shouldGateMute(n model.Node) bool {
	if n.Type != model.NodeTypeMute {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(n.Params.LogicKind))
	return kind != "" && kind != "none"
}

func incrementTriggerCount(counts map[model.NodeID]int, id model.NodeID) int {
	if counts == nil {
		return 0
	}
	counts[id]++
	return counts[id]
}

func (p *Predictor) shouldTriggerNode(row, idx int, bi model.BeatInfo, n model.Node, counts map[model.NodeID]int, lastFired *model.NodeID, lastTrig map[model.NodeID]bool) bool {
	if bi.NodeType != model.NodeTypeRegular && bi.NodeType != model.NodeTypeMute {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(n.Params.LogicKind))
	switch kind {
	case "", "none":
		return true
	case "every_n_triggers":
		if n.Params.LogicN > 0 {
			if n.Type == model.NodeTypeMute {
				cycleLen := len(p.beatInfosByRow[row])
				if cycleLen <= 0 {
					cycleLen = 1
				}
				cycle := idx / cycleLen
				if (cycle+1)%n.Params.LogicN != 0 {
					return false
				}
			} else {
				counts[bi.NodeID]++
				if counts[bi.NodeID]%n.Params.LogicN != 0 {
					return false
				}
			}
		}
	case "skip_every_n":
		if n.Params.LogicN > 0 {
			counts[bi.NodeID]++
			if counts[bi.NodeID]%n.Params.LogicN == 0 {
				return false
			}
		}
	case "probability":
		pval := n.Params.LogicP
		if pval <= 0 {
			return false
		}
		if pval < 1 {
			if p.deterministicRoll(row, idx, bi.NodeID) > pval {
				return false
			}
		}
	case "trigger_if_prev_skipped", "trigger_if_prev_triggered":
		j := idx - 1
		prevID := model.InvalidNodeID
		for k := 0; k < 64; k++ {
			prev := p.beatInfoAtRow(row, j)
			if prev.NodeType == model.NodeTypeRegular {
				prevID = prev.NodeID
				break
			}
			j--
		}
		prevTrig := false
		if prevID != model.InvalidNodeID {
			if v, ok := lastTrig[prevID]; ok {
				prevTrig = v
			} else if lastFired != nil {
				prevTrig = (*lastFired == prevID)
			}
		}
		switch kind {
		case "trigger_if_prev_skipped":
			if prevID == model.InvalidNodeID || prevTrig {
				return false
			}
		case "trigger_if_prev_triggered":
			if prevID == model.InvalidNodeID || !prevTrig {
				return false
			}
		}
	}
	return true
}

func (p *Predictor) evalAudible(row, idx int, bi model.BeatInfo, counts map[model.NodeID]int, triggerCounts map[model.NodeID]int, lastFired *model.NodeID, lastTrig map[model.NodeID]bool, gate *int) (bool, bool) {
	if bi.NodeType == model.NodeTypeMute {
		nNode, ok := p.nodes[bi.NodeID]
		if !ok {
			if lastTrig != nil {
				lastTrig[bi.NodeID] = false
			}
			return false, false
		}
		trigger := p.shouldTriggerNode(row, idx, bi, nNode, counts, lastFired, lastTrig)
		if trigger && nNode.Params.Logic != nil {
			count := incrementTriggerCount(triggerCounts, bi.NodeID)
			dec := nNode.Params.Logic(model.NodeContext{
				NodeID:        bi.NodeID,
				Row:           row,
				AbsoluteIndex: idx,
				TriggerCount:  count,
			})
			if dec.Enabled != nil && !*dec.Enabled {
				trigger = false
			}
		}
		if lastTrig != nil {
			lastTrig[bi.NodeID] = trigger
		}
		if trigger && gate != nil {
			// Gate covers only the mute subdivision plus any explicit hold.
			next := idx + muteHoldStepsFromNode(nNode) + 1
			if next > *gate {
				*gate = next
			}
		}
		return false, trigger
	}
	if bi.NodeType != model.NodeTypeRegular {
		if lastTrig != nil {
			lastTrig[bi.NodeID] = false
		}
		return false, false
	}
	if gate != nil && idx < *gate {
		if lastTrig != nil {
			lastTrig[bi.NodeID] = false
		}
		return false, false
	}
	nNode, ok := p.nodes[bi.NodeID]
	if !ok {
		if lastTrig != nil {
			lastTrig[bi.NodeID] = false
		}
		return false, false
	}
	if !p.shouldTriggerNode(row, idx, bi, nNode, counts, lastFired, lastTrig) {
		if lastTrig != nil {
			lastTrig[bi.NodeID] = false
		}
		return false, false
	}
	if nNode.Params.Logic != nil {
		count := incrementTriggerCount(triggerCounts, bi.NodeID)
		dec := nNode.Params.Logic(model.NodeContext{
			NodeID:        bi.NodeID,
			Row:           row,
			AbsoluteIndex: idx,
			TriggerCount:  count,
		})
		if dec.Enabled != nil && !*dec.Enabled {
			if lastTrig != nil {
				lastTrig[bi.NodeID] = false
			}
			return false, false
		}
	}
	if lastTrig != nil {
		lastTrig[bi.NodeID] = true
	}
	if lastFired != nil {
		*lastFired = bi.NodeID
	}
	return true, true
}

// deterministicRoll mirrors UI's hash-based roll for probability logic.
func (p *Predictor) deterministicRoll(row, idx int, id model.NodeID) float64 {
	x := uint64(uint32(row))<<32 ^ uint64(uint32(idx)) ^ (uint64(uint32(id)) << 16) ^ 0x9E3779B97F4A7C15
	x += 0x9E3779B97F4A7C15
	z := x
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	z ^= (z >> 31)
	return float64(z&((1<<53)-1)) / float64(1<<53)
}
