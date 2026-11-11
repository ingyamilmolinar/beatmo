package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func TestBuildRowWindow_DoesNotMutateTimeline(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.drum.Rows = []*DrumRow{{}}
	g.drum.SetLength(8)
	g.drum.Offset = 0
	g.beatInfosByRow = [][]model.BeatInfo{make([]model.BeatInfo, 8)}
	for i := range g.beatInfosByRow[0] {
		g.beatInfosByRow[0][i] = model.BeatInfo{NodeID: model.NodeID(i + 1), NodeType: model.NodeTypeRegular}
	}
	g.isLoopByRow = []bool{false}
	g.nextBeatIdxs = []int{2}    // abs>=2 is future
	g.frozenUpToByRow = []int{7} // freeze far ahead (bug-like)

	// Seed a leaked immutable commit in the future window.
	g.recordTimelineCommitKind(0, 5, false, model.NodeTypeRegular, timeline.CommitKindPlayback)
	_, _, kind, ok := g.timelineCommittedWithKind(0, 5)
	if !ok || kind != timeline.CommitKindPlayback {
		t.Fatalf("expected playback commit before build; ok=%v kind=%v", ok, kind)
	}

	cfg := rowWindowConfig{
		freezeLimit:     7,
		prevSteps:       make([]bool, 8),
		prevStepsOffset: 0,
		prevTypes:       make([]model.NodeType, 8),
		prevTypesOffset: 0,
		fastPath:        false,
		predictAt: func(abs int, bi model.BeatInfo) bool {
			_ = bi
			return abs == 5
		},
		reuseSteps: make([]bool, 8),
		reuseTypes: make([]model.NodeType, 8),
	}

	steps, _ := g.buildRowWindow(0, cfg)
	if !steps[5] {
		t.Fatalf("expected future playback commit to follow predictor=true; steps=%v", steps)
	}

	// Building the window must not mutate timeline provenance (demotion is a separate phase).
	_, _, kind, ok = g.timelineCommittedWithKind(0, 5)
	if !ok || kind != timeline.CommitKindPlayback {
		t.Fatalf("expected timeline kind unchanged after build; ok=%v kind=%v", ok, kind)
	}
}
