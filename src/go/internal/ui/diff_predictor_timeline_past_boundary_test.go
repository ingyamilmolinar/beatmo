package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

func TestDiffPredictorTimeline_SkipsRowPastBoundary(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)

	g.drum.Rows = []*DrumRow{{}}
	g.drum.SetLength(16)
	g.drum.Offset = 0

	abs := 10
	// Immutable playback commit disagrees with predictor.
	g.recordTimelineCommitKind(0, abs, true, model.NodeTypeRegular, timeline.CommitKindPlayback)

	// Simulate sequencer being ahead of UI: past boundary is seqNextIdxs.
	g.nextBeatIdxs = []int{0}
	g.seqNextIdxs = []int{abs + 1}

	if mismatches := g.diffPredictorTimeline(0, 0, 16, -1); len(mismatches) != 0 {
		t.Fatalf("expected no mismatches for abs < past boundary; got %v", mismatches)
	}
}

func TestDiffPredictorTimeline_ReportsFutureMismatch(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)

	g.drum.Rows = []*DrumRow{{}}
	g.drum.SetLength(16)
	g.drum.Offset = 0

	abs := 10
	g.recordTimelineCommitKind(0, abs, true, model.NodeTypeRegular, timeline.CommitKindPlayback)

	// No playhead boundary: mismatch should be reported.
	g.nextBeatIdxs = []int{0}
	g.seqNextIdxs = []int{0}

	mismatches := g.diffPredictorTimeline(0, 0, 16, -1)
	if len(mismatches) == 0 {
		t.Fatalf("expected mismatch reported")
	}
	found := false
	for _, m := range mismatches {
		if m.Abs == abs && m.Kind == "timeline_vs_pred" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected mismatch at abs=%d kind=timeline_vs_pred; got %v", abs, mismatches)
	}
}
