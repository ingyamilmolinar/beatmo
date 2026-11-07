package model

import "sort"

// GroupIndex is an immutable per-node view over group rules + membership,
// rebuilt on group mutation and read at trigger time. Readers must treat it
// as frozen; mutations swap in a freshly built index.
type GroupIndex struct {
	rulesByNode  map[NodeID][]GroupRule
	memberByNode map[NodeID]bool
}

// Folded in ascending GroupID order: stacking is order-independent, but iteration stays stable. Nil-safe.
func BuildGroupIndex(groups map[GroupID]*NodeGroup) *GroupIndex {
	idx := &GroupIndex{
		rulesByNode:  map[NodeID][]GroupRule{},
		memberByNode: map[NodeID]bool{},
	}
	ids := make([]GroupID, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		grp := groups[id]
		for _, n := range grp.NodeIDs {
			idx.memberByNode[n] = true
			// Only an enabled group's Rules feed the index; membership (ring rendering) ignores the flag.
			if grp.RulesEnabled && len(grp.Rules) > 0 {
				idx.rulesByNode[n] = append(idx.rulesByNode[n], grp.Rules...)
			}
		}
	}
	return idx
}

// Member reports whether n belongs to any group (for rendering rings).
func (idx *GroupIndex) Member(n NodeID) bool {
	if idx == nil {
		return false
	}
	return idx.memberByNode[n]
}

// HasRules reports whether any group rule affects n.
func (idx *GroupIndex) HasRules(n NodeID) bool {
	if idx == nil {
		return false
	}
	return len(idx.rulesByNode[n]) > 0
}

// The same cycle math as the mute node's every_n_triggers.
func GroupRound(idx, loopStart, loopLen int) int {
	if loopLen <= 0 || idx <= loopStart {
		return 0
	}
	return (idx - loopStart) / loopLen
}

func clampGroupF(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// Pitch deltas sum; vol/dur multiply, each rule clamp(1 + Delta×steps, Min, Max). Identity (0, 1, 1). steps =
// max(0, round(absIdx) − round(min(AnchorIdx, absIdx))) / EveryN: counted from the rule's own anchor, and a seek before
// the anchor is identity, never negative.
func GroupEffect(idx *GroupIndex, n NodeID, absIdx, loopStart, loopLen int) (float64, float64, float64) {
	pitchDelta, volMul, durMul := 0.0, 1.0, 1.0
	if idx == nil {
		return pitchDelta, volMul, durMul
	}
	round := GroupRound(absIdx, loopStart, loopLen)
	for _, r := range idx.rulesByNode[n] {
		anchor := r.AnchorIdx
		if anchor < 0 {
			anchor = 0
		}
		if absIdx < anchor {
			anchor = absIdx
		}
		anchorRound := GroupRound(anchor, loopStart, loopLen)
		steps := 0
		if r.EveryN > 0 {
			steps = (round - anchorRound) / r.EveryN
		}
		// Skip rule contribution when steps <= 0 to preserve identity (0 for
		// pitch, 1 for vol/dur).
		if steps <= 0 {
			continue
		}
		switch r.Param {
		case GroupParamPitch:
			pitchDelta += clampGroupF(r.Delta*float64(steps), r.Min, r.Max)
		case GroupParamVolume:
			volMul *= clampGroupF(1+r.Delta*float64(steps), r.Min, r.Max)
		case GroupParamDuration:
			durMul *= clampGroupF(1+r.Delta*float64(steps), r.Min, r.Max)
		}
	}
	return pitchDelta, volMul, durMul
}
