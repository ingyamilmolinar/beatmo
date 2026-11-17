package ui

import "github.com/ingyamilmolinar/beatmo/core/model"

type seqPathSnapshot struct {
	beatInfosByRow [][]model.BeatInfo
	isLoopByRow    []bool
	loopStartByRow []int
}

func (g *Game) storeSeqPathSnapshot() {
	if g == nil {
		return
	}
	snap := &seqPathSnapshot{
		beatInfosByRow: cloneBeatInfosByRow(g.beatInfosByRow),
		isLoopByRow:    append([]bool(nil), g.isLoopByRow...),
		loopStartByRow: append([]int(nil), g.loopStartByRow...),
	}
	g.seqPathSnap.Store(snap)
}

func (g *Game) seqPathSnapshot() *seqPathSnapshot {
	if g == nil {
		return nil
	}
	return g.seqPathSnap.Load()
}

func cloneBeatInfosByRow(src [][]model.BeatInfo) [][]model.BeatInfo {
	if len(src) == 0 {
		return nil
	}
	out := make([][]model.BeatInfo, len(src))
	for i := range src {
		if len(src[i]) == 0 {
			continue
		}
		out[i] = append([]model.BeatInfo(nil), src[i]...)
	}
	return out
}

func seqBeatInfoAtRow(snap *seqPathSnapshot, row, idx int) model.BeatInfo {
	if snap == nil || row < 0 || row >= len(snap.beatInfosByRow) {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	infos := snap.beatInfosByRow[row]
	if len(infos) == 0 || idx < 0 {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	if idx < len(infos) {
		return infos[idx]
	}
	if row >= len(snap.isLoopByRow) || !snap.isLoopByRow[row] {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	start := 0
	if row < len(snap.loopStartByRow) {
		start = snap.loopStartByRow[row]
	}
	loopLen := len(infos) - start
	if loopLen <= 0 {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	rel := idx - start
	rel %= loopLen
	if rel < 0 {
		rel += loopLen
	}
	return infos[start+rel]
}
