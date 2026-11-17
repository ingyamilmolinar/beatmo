package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// Regression: immutable timeline commits must never mask the mutable future
// beyond the current playhead. If nextBeatIdxs/seqNextIdxs get ahead (e.g., via
// stale freezes during live edits), DrumView must still rebuild from the true
// present into the future so UI parity matches the engine predictor.
func TestDrumView_ImmutableCommitBeyondPlayheadDoesNotMaskFuture(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)

	// Build a simple loop so predictor.VisibleAt is deterministically true.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(1, 1, model.NodeTypeRegular)
	d := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a

	g.drum.SetLength(64)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Simulate playback at abs=10 (so the true future starts at 11).
	g.SetPlaying(true)
	g.elapsedBeats = 10

	// Corrupt the per-row "next" indices far into the future. This should not
	// cause commits beyond the playhead to be treated as immutable past.
	rows := len(g.drum.Rows)
	g.nextBeatIdxs = make([]int, rows)
	g.seqNextIdxs = make([]int, rows)
	for i := 0; i < rows; i++ {
		g.nextBeatIdxs[i] = 50
		g.seqNextIdxs[i] = 50
	}

	// Plant an immutable commit in the future window that contradicts the
	// predictor. Without proper playhead clamping this masks the window until
	// it scrolls out.
	const row = 0
	const abs = 20
	g.recordTimelineCommitKind(row, abs, false, model.NodeTypeInvisible, timeline.CommitKindPlayback)

	g.engine.Predictor.Ensure(abs + 1)
	want := g.engine.Predictor.VisibleAt(row, abs)
	if !want {
		t.Fatalf("sanity: predictor expected visible at abs=%d", abs)
	}

	g.drum.Offset = 0
	g.refreshDrumRow()

	idx := abs - g.drum.Offset
	if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
		t.Fatalf("abs %d not in window offset=%d len=%d", abs, g.drum.Offset, len(g.drum.Rows[row].Steps))
	}
	if got := g.drum.Rows[row].Steps[idx]; got != want {
		_, typ, kind, ok := g.timelineCommittedWithKind(row, abs)
		t.Fatalf("future immutable commit masked view: abs=%d want=%v got=%v commitTyp=%v kind=%v ok=%v next=%v seqNext=%v playhead=%d",
			abs, want, got, typ, kind, ok, g.nextBeatIdxs, g.seqNextIdxs, g.elapsedBeats)
	}

	// The leaked immutable commit should be demoted to Released so it remains
	// debuggable but does not enforce immutability in the future window.
	_, _, kind, ok := g.timelineCommittedWithKind(row, abs)
	if !ok {
		t.Fatalf("expected commit retained after demotion")
	}
	if kind != timeline.CommitKindReleased {
		t.Fatalf("expected future commit demoted to Released; got %v", kind)
	}
}
