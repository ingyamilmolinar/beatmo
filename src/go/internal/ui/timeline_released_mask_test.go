package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

// Regression: released timeline commits should not suppress predictor values
// for future windows after a path change. In an earlier bug, steps inherited
// false from the previous window even though the released commit and predictor
// both said true.
func TestBuildRowWindowReleasedCommitUsesPredictor(t *testing.T) {
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
	g.nextBeatIdxs = []int{6} // playhead is ahead of window start
	g.frozenUpToByRow = []int{7} // freeze a bit past the index
	g.drumBeatInfos = make([]model.BeatInfo, g.drum.Length)

	// Seed a released commit at abs=5 with value=true to mirror prior playback.
	g.recordTimelineCommitKind(0, 5, true, model.NodeTypeRegular, timeline.CommitKindReleased)

	cfg := rowWindowConfig{
		freezeLimit:     7,
		prevSteps:       make([]bool, 8), // all false so any overwrite would mask the predictor
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
		t.Fatalf("expected released commit at abs=5 to follow predictor=true; got false (steps=%v)", steps)
	}
}
