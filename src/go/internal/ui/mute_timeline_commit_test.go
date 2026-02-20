package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func TestApplySequencerHighlight_CommitsMuteAsOn(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlaying(true)

	// Ensure a single row exists.
	if len(g.drum.Rows) == 0 {
		t.Fatalf("expected default drum rows")
	}
	// Build a simple loop with a mute node so beat info matches the highlight.
	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	mute := g.tryAddNode(1, 0, model.NodeTypeMute)
	g.start = start
	g.graph.StartNodeID = start.ID
	g.addEdge(start, mute)
	g.addEdge(mute, start)
	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	g.updateBeatInfos()
	if len(g.beatInfosByRow) == 0 || len(g.beatInfosByRow[0]) == 0 {
		t.Fatalf("beat infos not generated")
	}
	muteIdx := -1
	var info model.BeatInfo
	for i, bi := range g.beatInfosByRow[0] {
		if bi.NodeID == mute.ID {
			muteIdx = i
			info = bi
			break
		}
	}
	if muteIdx < 0 {
		t.Fatalf("mute beat not found in beat infos")
	}
	g.applySequencerHighlight(0, muteIdx, info)

	val, typ, kind, ok := g.timelineCommittedWithKind(0, muteIdx)
	if !ok {
		t.Fatalf("expected timeline commit for mute highlight")
	}
	if kind != timeline.CommitKindPlayback {
		t.Fatalf("expected playback commit kind, got %v", kind)
	}
	if typ != model.NodeTypeMute {
		t.Fatalf("expected mute commit type, got %v", typ)
	}
	if !val {
		t.Fatalf("expected mute commit to be ON when triggered; got false")
	}
}
