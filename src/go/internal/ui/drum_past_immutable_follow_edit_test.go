package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Highlights arrive asynchronously and an edit drops the in-flight ones, so the row's index lags the playhead; the
// immutable boundary is the VISUAL playhead, or cells in [lagged, playhead] follow the edited predictor.
func TestDrumView_PastImmutableWhenHighlightLagsPlayheadDuringEdit(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.Layout(1024, 720)

	// 4-node loop for row 0.
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

	g.drum.SetLength(24)
	g.drum.SetFollow(false)
	g.updateBeatInfos()
	g.refreshDrumRow()

	dst := ebiten.NewImage(1024, 720)
	g.drum.Draw(dst, nil, 0, nil, 0)
	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)

	// Play far enough to render and freeze a solid block of history.
	g.SetPlaying(true)
	const playhead = 16
	for abs := 0; abs <= playhead; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	g.elapsedBeats = playhead
	g.refreshDrumRow()
	g.drum.Draw(dst, nil, 0, nil, 0)

	row := 0
	off := g.drum.Offset

	// Snapshot the visible past (everything strictly left of the playhead).
	pastSteps := make(map[int]bool)
	pastTypes := make(map[int]model.NodeType)
	for abs := off; abs <= playhead && abs < off+g.drum.Length; abs++ {
		idx := abs - off
		if idx >= 0 && idx < len(g.drum.Rows[row].Steps) {
			pastSteps[abs] = g.drum.Rows[row].Steps[idx]
			pastTypes[abs] = g.drum.Rows[row].CellTypes[idx]
		}
	}
	if len(pastSteps) < 4 {
		t.Fatalf("expected a block of visible past cells, got %d (off=%d playhead=%d)", len(pastSteps), off, playhead)
	}

	// The post-edit lag: indices behind the playhead, elapsedBeats at it.
	const lagged = 6
	if len(g.nextBeatIdxs) <= row || len(g.seqNextIdxs) <= row {
		t.Fatalf("missing per-row indices")
	}
	g.nextBeatIdxs[row] = lagged
	g.seqNextIdxs[row] = lagged

	// A detour through a new node changes the real future; the past must not follow.
	n1 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.deleteEdge(b, c)
	g.addEdge(b, n1)
	g.addEdge(n1, c)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.Draw(dst, nil, 0, nil, 0)

	off1 := g.drum.Offset
	for abs, wantStep := range pastSteps {
		idx := abs - off1
		if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
			continue
		}
		if got := g.drum.Rows[row].Steps[idx]; got != wantStep {
			t.Fatalf("PAST STEP CHANGED on edit (highlight lagged playhead): abs=%d idx=%d want=%v got=%v lagged=%d playhead=%d off=%d",
				abs, idx, wantStep, got, lagged, playhead, off1)
		}
		if got := g.drum.Rows[row].CellTypes[idx]; got != pastTypes[abs] {
			t.Fatalf("PAST TYPE CHANGED on edit (highlight lagged playhead): abs=%d idx=%d want=%v got=%v lagged=%d playhead=%d",
				abs, idx, pastTypes[abs], got, lagged, playhead)
		}
	}
}
