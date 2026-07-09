package ui

import "github.com/ingyamilmolinar/beatmo/core/model"

// refreshGroupIndex rebuilds the immutable group-rule index from the graph.
// Called from the graph's group-changed hook (UI thread) and after import.
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

// rowLoopSpan maps a row to its (loopStart, loopLen) pair for GroupRound/
// GroupEffect. Non-loop rows (or out-of-range rows) return (0, 0), which
// GroupRound treats as "always round 0" (loopLen<=0 guard). Mirrors
// game_graph_beat_index.go's loop math; callers hold seqMu (sequencer
// schedule / Update), matching how isLoopByRow is read elsewhere.
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

// ruleAnchorIdxNow is the single capture point for "the absolute subdivision
// index a group rule is anchored to right now": the canonical playhead
// (playheadAbsSubdiv, game_seek.go — the same clock the mini-timeline cursor
// and drum-view auto-scroll derive from) while playing, 0 (the timeline
// origin) otherwise. Every UI/JS path that enables or rewrites a group rule
// must stamp GroupRule.AnchorIdx via this accessor so the rule's step count
// starts from the moment it was set, not from the timeline origin (see
// GroupEffect's doc comment for the root-cause bug this fixes).
func (g *Game) ruleAnchorIdxNow() int {
	if !g.Playing() {
		return 0
	}
	return g.playheadAbsSubdiv()
}

// onGroupsChanged reacts to any group mutation: rebuild the index and, while
// playing, drop in-flight audio + advance the parity generation so the new
// rule takes effect from the next re-predicted trigger (same mechanism as the
// node-param-change hook in game_new.go).
func (g *Game) onGroupsChanged() {
	g.refreshGroupIndex()
	if g.importing {
		return
	}
	if g.groupMenu != nil && g.groupMenu.IsOpen() {
		if _, ok := g.graph.Group(g.groupMenu.GroupID()); !ok {
			g.groupMenu.Close()
		}
	}
	if g.Playing() {
		g.audioGen.Add(1)
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
