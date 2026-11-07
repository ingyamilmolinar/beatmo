package ui

import "github.com/ingyamilmolinar/beatmo/core/model"

// UI thread only: from the group-changed hook and after import.
func (g *Game) refreshGroupIndex() {
	idx := model.BuildGroupIndex(g.graph.Groups)
	g.groupIdxMu.Lock()
	g.groupIdx = idx
	g.groupIdxMu.Unlock()
}

// groupIndexSnapshot returns the current immutable index (may be nil).
func (g *Game) groupIndexSnapshot() *model.GroupIndex {
	g.groupIdxMu.RLock()
	defer g.groupIdxMu.RUnlock()
	return g.groupIdx
}

// (0, 0) for non-loop or out-of-range rows, which GroupRound treats as round 0. Callers hold seqMu.
func (g *Game) rowLoopSpan(row int) (loopStart, loopLen int) {
	if row < 0 || row >= len(g.isLoopByRow) || !g.isLoopByRow[row] {
		return 0, 0
	}
	if row >= len(g.loopStartByRow) || row >= len(g.beatInfosByRow) {
		return 0, 0
	}
	start := g.loopStartByRow[row]
	return start, len(g.beatInfosByRow[row]) - start
}

// Every path that enables or rewrites a rule stamps AnchorIdx through this, so the rule counts
// steps from when it was set, not from the timeline origin.
func (g *Game) ruleAnchorIdxNow() int {
	if !g.Playing() {
		return 0
	}
	return g.playheadAbsSubdiv()
}

// While playing, drop in-flight audio and advance parity so the new rule applies from the next
// re-predicted trigger.
func (g *Game) onGroupsChanged() {
	g.refreshGroupIndex()
	g.laneGraphVersion.Add(1) // group rules change note params: re-fingerprint the audio lane
	if g.importing {
		return
	}
	if g.groupMenu != nil && g.groupMenu.IsOpen() {
		if _, ok := g.graph.Group(g.groupMenu.GroupID()); !ok {
			g.groupMenu.Close()
		}
	}
	if g.Playing() {
		// The lane reconciles through laneGraphVersion; a transport-generation bump would cancel-all and
		// re-commit, doubling a note inside the cancel guard.
		if !g.audioLaneOn() {
			g.audioGen.Add(1)
		}
		g.bumpParityGen("group-rule-change", structuralMutationOptions{
			SkipPathsDirty:     true,
			SkipPathChangeMark: true,
			SkipBufferClear:    true,
		})
		g.parityMu.Lock()
		g.parityAudio = nil
		g.parityAudioMaxIdx = nil
		g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
		g.parityMu.Unlock()
		g.ClearParityMismatches()
	}
}
