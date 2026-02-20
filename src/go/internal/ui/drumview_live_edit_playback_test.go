package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Regression: deleting + re-adding a node during playback must update the
// DrumView window immediately for abs>=pastExclusive (past is immutable).
func TestDrumView_LiveReaddDuringPlaybackUpdatesFutureWindow(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(1024, 720)

	// Small loop with adjacent nodes (no synthesized invisible steps) so
	// beatInfo neighbors are stable for reconnect.
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

	g.drum.SetLength(64)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Start playback and advance enough steps so we have a meaningful future window.
	pressPlay(t, g.drum)
	advancePlaybackByAbs(g, g.grid.MaxDiv()*2)

	row := 0
	if row >= len(g.nextBeatIdxs) {
		t.Fatalf("missing nextBeatIdxs for row %d", row)
	}

	// Pick a target a few steps into the mutable future, but still within view.
	pastExclusive := g.nextBeatIdxs[row]
	if row < len(g.seqNextIdxs) && g.seqNextIdxs[row] > pastExclusive {
		pastExclusive = g.seqNextIdxs[row]
	}
	// Snapshot the past portion of the visible window so we can assert it remains
	// immutable across the edit (only abs>=pastExclusive may change).
	off0 := g.drum.Offset
	pastEnd0 := pastExclusive
	if max := off0 + g.drum.Length; pastEnd0 > max {
		pastEnd0 = max
	}
	pastSteps0 := make(map[int]bool)
	pastTypes0 := make(map[int]model.NodeType)
	for abs := off0; abs < pastEnd0; abs++ {
		idx := abs - off0
		if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
			continue
		}
		pastSteps0[abs] = g.drum.Rows[row].Steps[idx]
		pastTypes0[abs] = g.drum.Rows[row].CellTypes[idx]
	}
	targetAbs := pastExclusive + 2
	if targetAbs < g.drum.Offset {
		targetAbs = g.drum.Offset
	}
	windowEnd := g.drum.Offset + g.drum.Length
	if targetAbs >= windowEnd {
		targetAbs = windowEnd - 1
	}
	if targetAbs < 0 {
		targetAbs = 0
	}

	bi := g.beatInfoAtRow(row, targetAbs)
	if bi.NodeID == model.InvalidNodeID {
		t.Fatalf("invalid beat info at row=%d abs=%d (offset=%d len=%d)", row, targetAbs, g.drum.Offset, g.drum.Length)
	}
	prev := g.beatInfoAtRow(row, targetAbs-1)
	next := g.beatInfoAtRow(row, targetAbs+1)
	n := g.nodeByID(bi.NodeID)
	if n == nil {
		t.Fatalf("node missing for beat info id=%d row=%d abs=%d", bi.NodeID, row, targetAbs)
	}

	// Build baseline row cache so stale-render regressions would be visible.
	dst := ebiten.NewImage(800, 240)
	g.drum.Draw(dst, nil, 0, nil, 0)
	if len(g.drum.rowCacheGen) == 0 {
		t.Fatalf("expected row cache generation tracking")
	}
	gen0 := g.drum.rowCacheGen[0]

	// Simulate the "follow" scroll happening in the same frame as the edit so
	// cache rebuilds can't hide behind a dx==0 full redraw.
	g.drum.Offset = g.drum.Offset + 1
	g.drum.markRowsShiftDirty()

	// Delete the node at targetAbs and re-add as silent (visible change) with
	// the same neighbors to keep the circuit intact.
	g.deleteNode(n)
	g.pendingStartRow = row
	re := g.tryAddNode(bi.I, bi.J, model.NodeTypeSilent)
	g.pendingStartRow = -1
	if p := g.nodeByID(prev.NodeID); p != nil {
		g.addEdge(p, re)
	}
	if nx := g.nodeByID(next.NodeID); nx != nil {
		g.addEdge(re, nx)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Rebuild caches: the edited row must force a full rebuild so changes are
	// visible immediately (not only in the newly revealed strip on the right).
	g.drum.Draw(dst, nil, 0, nil, 0)
	if g.drum.rowCacheGen[0] == gen0 {
		t.Fatalf("row cache reused sprite after live re-add during shift; gen=%d", gen0)
	}

	// Ensure predictor covers the current visible window.
	horizon := g.drum.Offset + g.drum.Length
	g.engine.Predictor.Ensure(horizon)

	// Past immutability: abs<pastExclusive must remain unchanged.
	off1 := g.drum.Offset
	pastEnd1 := pastExclusive
	if max := off1 + g.drum.Length; pastEnd1 > max {
		pastEnd1 = max
	}
	for abs := off1; abs < pastEnd1; abs++ {
		wantStep, okStep := pastSteps0[abs]
		wantTyp, okTyp := pastTypes0[abs]
		if !okStep || !okTyp {
			continue
		}
		idx := abs - off1
		if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
			continue
		}
		if got := g.drum.Rows[row].Steps[idx]; got != wantStep {
			t.Fatalf("past mutated after live re-add: row=%d abs=%d idx=%d want=%v got=%v pastExclusive=%d off0=%d off1=%d",
				row, abs, idx, wantStep, got, pastExclusive, off0, off1)
		}
		if got := g.drum.Rows[row].CellTypes[idx]; got != wantTyp {
			t.Fatalf("past type mutated after live re-add: row=%d abs=%d idx=%d want=%v got=%v pastExclusive=%d off0=%d off1=%d",
				row, abs, idx, wantTyp, got, pastExclusive, off0, off1)
		}
	}

	// From the true past boundary onward, DrumView should mirror predictor truth.
	for abs := g.drum.Offset; abs < g.drum.Offset+g.drum.Length; abs++ {
		if abs < pastExclusive {
			continue
		}
		idx := abs - g.drum.Offset
		if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
			continue
		}
		info := g.beatInfoAtRow(row, abs)
		want := false
		if info.NodeType == model.NodeTypeMute {
			want = g.engine.Predictor.TriggeredAt(row, abs)
		} else {
			want = g.engine.Predictor.VisibleAt(row, abs)
		}
		if got := g.drum.Rows[row].Steps[idx]; got != want {
			t.Fatalf("drum view mismatch after live re-add: row=%d abs=%d idx=%d want=%v got=%v pastExclusive=%d offset=%d next=%v seqNext=%v",
				row, abs, idx, want, got, pastExclusive, g.drum.Offset, g.nextBeatIdxs, g.seqNextIdxs)
		}
		if gotTyp := g.drum.Rows[row].CellTypes[idx]; gotTyp != info.NodeType {
			t.Fatalf("drum view type mismatch after live re-add: row=%d abs=%d idx=%d want=%v got=%v pastExclusive=%d offset=%d next=%v",
				row, abs, idx, info.NodeType, gotTyp, pastExclusive, g.drum.Offset, g.nextBeatIdxs)
		}
	}
}

func TestDrumView_LiveReaddNonPrimaryRowDuringPlaybackUpdatesFutureWindow(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(1024, 720)

	// Row 0 circuit.
	a0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b0 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c0 := g.tryAddNode(1, 1, model.NodeTypeRegular)
	d0 := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a0, b0)
	g.addEdge(b0, c0)
	g.addEdge(c0, d0)
	g.addEdge(d0, a0)
	g.start = a0
	g.graph.StartNodeID = a0.ID

	// Row 1 circuit (disjoint).
	for len(g.drum.Rows) < 2 {
		g.drum.AddRow()
	}
	a1 := g.tryAddNode(3, 0, model.NodeTypeRegular)
	b1 := g.tryAddNode(4, 0, model.NodeTypeRegular)
	c1 := g.tryAddNode(4, 1, model.NodeTypeRegular)
	d1 := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.addEdge(a1, b1)
	g.addEdge(b1, c1)
	g.addEdge(c1, d1)
	g.addEdge(d1, a1)

	g.drum.Rows[1].Origin = a1.ID
	g.drum.Rows[1].Node = a1

	g.drum.SetLength(64)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Start playback and advance enough steps so we have a meaningful future window.
	pressPlay(t, g.drum)
	advancePlaybackByAbs(g, g.grid.MaxDiv()*2)

	row := 1
	if row >= len(g.nextBeatIdxs) {
		t.Fatalf("missing nextBeatIdxs for row %d", row)
	}

	// Ensure baseline row caches are built.
	dst := ebiten.NewImage(800, 240)
	g.drum.Draw(dst, nil, 0, nil, 0)
	if len(g.drum.rowCacheGen) <= row {
		t.Fatalf("expected row cache generation tracking for row %d", row)
	}
	gen0 := g.drum.rowCacheGen[row]

	// Simulate the "follow" scroll happening in the same frame as the edit.
	g.drum.Offset = g.drum.Offset + 1
	g.drum.markRowsShiftDirty()

	// Choose a future abs whose node is not the row origin (deleteNode removes origin rows).
	pastExclusive := g.nextBeatIdxs[row]
	if row < len(g.seqNextIdxs) && g.seqNextIdxs[row] > pastExclusive {
		pastExclusive = g.seqNextIdxs[row]
	}
	// Snapshot past portion of the window for immutability checks after the edit.
	off0 := g.drum.Offset
	pastEnd0 := pastExclusive
	if max := off0 + g.drum.Length; pastEnd0 > max {
		pastEnd0 = max
	}
	pastSteps0 := make(map[int]bool)
	pastTypes0 := make(map[int]model.NodeType)
	for abs := off0; abs < pastEnd0; abs++ {
		idx := abs - off0
		if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
			continue
		}
		pastSteps0[abs] = g.drum.Rows[row].Steps[idx]
		pastTypes0[abs] = g.drum.Rows[row].CellTypes[idx]
	}
	windowEnd := g.drum.Offset + g.drum.Length
	targetAbs := -1
	for abs := pastExclusive + 1; abs < windowEnd; abs++ {
		info := g.beatInfoAtRow(row, abs)
		if info.NodeID != model.InvalidNodeID && info.NodeID != g.drum.Rows[row].Origin {
			targetAbs = abs
			break
		}
	}
	if targetAbs < 0 {
		t.Fatalf("no suitable future node found in window for row %d (pastExclusive=%d offset=%d len=%d)", row, pastExclusive, g.drum.Offset, g.drum.Length)
	}

	bi := g.beatInfoAtRow(row, targetAbs)
	prev := g.beatInfoAtRow(row, targetAbs-1)
	next := g.beatInfoAtRow(row, targetAbs+1)
	n := g.nodeByID(bi.NodeID)
	if n == nil {
		t.Fatalf("node missing for beat info id=%d row=%d abs=%d", bi.NodeID, row, targetAbs)
	}

	// Delete + re-add as silent to ensure a visible change.
	g.deleteNode(n)
	g.pendingStartRow = row
	re := g.tryAddNode(bi.I, bi.J, model.NodeTypeSilent)
	g.pendingStartRow = -1
	if p := g.nodeByID(prev.NodeID); p != nil {
		g.addEdge(p, re)
	}
	if nx := g.nodeByID(next.NodeID); nx != nil {
		g.addEdge(re, nx)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Rebuild caches: edited row must force a full rebuild so changes are visible immediately.
	g.drum.Draw(dst, nil, 0, nil, 0)
	if g.drum.rowCacheGen[row] == gen0 {
		t.Fatalf("row %d cache reused sprite after live re-add during shift; gen=%d", row, gen0)
	}

	// Ensure predictor covers the current visible window and DrumView mirrors it in the mutable future.
	horizon := g.drum.Offset + g.drum.Length
	g.engine.Predictor.Ensure(horizon)
	off1 := g.drum.Offset
	pastEnd1 := pastExclusive
	if max := off1 + g.drum.Length; pastEnd1 > max {
		pastEnd1 = max
	}
	for abs := off1; abs < pastEnd1; abs++ {
		wantStep, okStep := pastSteps0[abs]
		wantTyp, okTyp := pastTypes0[abs]
		if !okStep || !okTyp {
			continue
		}
		idx := abs - off1
		if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
			continue
		}
		if got := g.drum.Rows[row].Steps[idx]; got != wantStep {
			t.Fatalf("past mutated after live re-add: row=%d abs=%d idx=%d want=%v got=%v pastExclusive=%d off0=%d off1=%d",
				row, abs, idx, wantStep, got, pastExclusive, off0, off1)
		}
		if got := g.drum.Rows[row].CellTypes[idx]; got != wantTyp {
			t.Fatalf("past type mutated after live re-add: row=%d abs=%d idx=%d want=%v got=%v pastExclusive=%d off0=%d off1=%d",
				row, abs, idx, wantTyp, got, pastExclusive, off0, off1)
		}
	}
	for abs := g.drum.Offset; abs < g.drum.Offset+g.drum.Length; abs++ {
		if abs < pastExclusive {
			continue
		}
		idx := abs - g.drum.Offset
		if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
			continue
		}
		info := g.beatInfoAtRow(row, abs)
		want := false
		if info.NodeType == model.NodeTypeMute {
			want = g.engine.Predictor.TriggeredAt(row, abs)
		} else {
			want = g.engine.Predictor.VisibleAt(row, abs)
		}
		if got := g.drum.Rows[row].Steps[idx]; got != want {
			t.Fatalf("drum view mismatch after live re-add: row=%d abs=%d idx=%d want=%v got=%v pastExclusive=%d offset=%d next=%v seqNext=%v",
				row, abs, idx, want, got, pastExclusive, g.drum.Offset, g.nextBeatIdxs, g.seqNextIdxs)
		}
		if gotTyp := g.drum.Rows[row].CellTypes[idx]; gotTyp != info.NodeType {
			t.Fatalf("drum view type mismatch after live re-add: row=%d abs=%d idx=%d want=%v got=%v pastExclusive=%d offset=%d next=%v",
				row, abs, idx, info.NodeType, gotTyp, pastExclusive, g.drum.Offset, g.nextBeatIdxs)
		}
	}
}

func TestDrumView_LiveInsertDuringPlaybackUpdatesFutureWindow(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(1024, 720)

	// Circuit with longer orthogonal edges so the traversal includes synthesized
	// invisible steps that can be turned into an explicit node by insertion.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 0, model.NodeTypeRegular)
	c := g.tryAddNode(4, 4, model.NodeTypeRegular)
	d := g.tryAddNode(0, 4, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID

	g.drum.SetLength(96)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Start playback and advance enough steps so we have a meaningful future window.
	pressPlay(t, g.drum)
	advancePlaybackByAbs(g, g.grid.MaxDiv()*2)

	row := 0
	pastExclusive := g.nextBeatIdxs[row]
	if row < len(g.seqNextIdxs) && g.seqNextIdxs[row] > pastExclusive {
		pastExclusive = g.seqNextIdxs[row]
	}
	before := append([]bool(nil), g.drum.Rows[row].Steps...)

	// Ensure baseline caches are built.
	dst := ebiten.NewImage(800, 240)
	g.drum.Draw(dst, nil, 0, nil, 0)
	if len(g.drum.rowCacheGen) == 0 {
		t.Fatalf("expected row cache generation tracking")
	}
	gen0 := g.drum.rowCacheGen[row]

	// Simulate follow scroll in the same frame as the insertion.
	g.drum.Offset = g.drum.Offset + 1
	g.drum.markRowsShiftDirty()

	// Insert a node along the A->B edge (at 2,0). tryAddNode will auto-stitch
	// that edge into A->new->B and refresh paths.
	g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Cache must fully rebuild so the inserted node's visible effect doesn't
	// appear only at the right edge during incremental scrolling.
	g.drum.Draw(dst, nil, 0, nil, 0)
	if g.drum.rowCacheGen[row] == gen0 {
		t.Fatalf("row cache reused sprite after live insert during shift; gen=%d", gen0)
	}

	after := g.drum.Rows[row].Steps
	// The insertion should affect at least one cell in the mutable future window.
	futureStart := pastExclusive - g.drum.Offset
	if futureStart < 0 {
		futureStart = 0
	}
	changed := false
	for i := futureStart; i < len(before) && i < len(after); i++ {
		if before[i] != after[i] {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatalf("expected at least one future cell to change after insertion; pastExclusive=%d offset=%d len=%d", pastExclusive, g.drum.Offset, g.drum.Length)
	}

	// Future should mirror predictor truth.
	horizon := g.drum.Offset + g.drum.Length
	g.engine.Predictor.Ensure(horizon)
	for abs := g.drum.Offset; abs < g.drum.Offset+g.drum.Length; abs++ {
		if abs < pastExclusive {
			continue
		}
		idx := abs - g.drum.Offset
		if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
			continue
		}
		info := g.beatInfoAtRow(row, abs)
		want := false
		if info.NodeType == model.NodeTypeMute {
			want = g.engine.Predictor.TriggeredAt(row, abs)
		} else {
			want = g.engine.Predictor.VisibleAt(row, abs)
		}
		if got := g.drum.Rows[row].Steps[idx]; got != want {
			t.Fatalf("drum view mismatch after live insert: row=%d abs=%d idx=%d want=%v got=%v pastExclusive=%d offset=%d next=%v seqNext=%v",
				row, abs, idx, want, got, pastExclusive, g.drum.Offset, g.nextBeatIdxs, g.seqNextIdxs)
		}
	}
}
