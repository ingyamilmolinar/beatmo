package ui

import (
	"sync/atomic"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

func ensureRowInstrumentsAvailable(t *testing.T, g *Game) {
	t.Helper()
	if g == nil || g.drum == nil {
		t.Fatalf("missing drum view while ensuring row instruments")
	}
	t.Cleanup(func() {
		audio.ResetInstruments()
		audio.ResetCatalogForTest(nil)
	})
	g.drum.refreshInstruments()
	for _, row := range g.drum.Rows {
		if row == nil || row.Instrument == "" {
			continue
		}
		if !g.drum.IsInstrumentAvailable(row.Instrument) {
			_ = audio.RegisterWAV(row.Instrument, "test://placeholder.wav")
		}
	}
	g.drum.refreshInstruments()
	for _, row := range g.drum.Rows {
		if row == nil || row.Instrument == "" {
			continue
		}
		if !g.drum.IsInstrumentAvailable(row.Instrument) {
			t.Fatalf("instrument %q still unavailable after registration", row.Instrument)
		}
	}
}

func driveSchedulerToAbs(t *testing.T, g *Game, row, abs int) {
	t.Helper()
	if g == nil {
		return
	}
	if abs < 0 {
		abs = 0
	}
	if !g.Playing() {
		g.SetPlaying(true)
	}
	iters := abs/8 + 4
	if iters < 4 {
		iters = 4
	}
	for i := 0; i < iters; i++ {
		scheduleAbsForMuteTest(g, abs)
		if row >= 0 && row < len(g.seqNextIdxs) && g.seqNextIdxs[row] > abs {
			return
		}
	}
	if row >= 0 && row < len(g.seqNextIdxs) && g.seqNextIdxs[row] <= abs {
		t.Fatalf("scheduler did not reach abs %d (seqNextIdxs=%v)", abs, g.seqNextIdxs)
	}
}

// Regression guard for playback parity when re-adding a node on a non-primary
// row while the sequencer is running and history is frozen. DrumView should
// mirror the engine predictor after a delete+re-add, rather than keeping a
// stale mask even though the predictor says the step is on.
func TestImportPlaybackReaddKeepsPredictorParity(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)
	g.drum.SetLength(16)

	buildRow := func(row int, x int) {
		g.pendingStartRow = row
		a := g.tryAddNode(x, 0, model.NodeTypeRegular)
		b := g.tryAddNode(x+1, 0, model.NodeTypeRegular)
		c := g.tryAddNode(x+1, 1, model.NodeTypeRegular)
		d := g.tryAddNode(x, 1, model.NodeTypeRegular)
		g.addEdge(a, b)
		g.addEdge(b, c)
		g.addEdge(c, d)
		g.addEdge(d, a)
		if row == 0 {
			g.start = a
			g.graph.StartNodeID = a.ID
		}
		g.pendingStartRow = -1
	}
	// Build five disjoint squares so row 4 is non-primary.
	for len(g.drum.Rows) < 5 {
		g.drum.AddRow()
	}
	for r := 0; r < 5; r++ {
		buildRow(r, r*3)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()

	row := 4
	if row >= len(g.beatInfosByRow) || len(g.beatInfosByRow[row]) == 0 {
		t.Fatalf("row %d missing beat infos", row)
	}
	if row >= len(g.drum.Rows) {
		t.Fatalf("expected at least %d rows, got %d", row+1, len(g.drum.Rows))
	}
	originID := g.drum.Rows[row].Origin
	if originID == model.InvalidNodeID {
		t.Fatalf("row %d missing origin node", row)
	}
	path := g.beatInfosByRow[row]
	targetAbs := -1
	var bi model.BeatInfo
	for i, info := range path {
		if info.NodeID != model.InvalidNodeID && info.NodeType == model.NodeTypeRegular && info.NodeID != originID {
			targetAbs = i
			bi = info
			break
		}
	}
	if targetAbs < 0 {
		t.Fatalf("row %d has no regular nodes in beat path", row)
	}
	// Find the nearest previous/next real nodes for reconnection.
	prev := model.BeatInfo{NodeID: model.InvalidNodeID}
	for i := targetAbs - 1; i >= 0; i-- {
		if path[i].NodeID != model.InvalidNodeID {
			prev = path[i]
			break
		}
	}
	next := model.BeatInfo{NodeID: model.InvalidNodeID}
	for i := targetAbs + 1; i < len(path); i++ {
		if path[i].NodeID != model.InvalidNodeID {
			next = path[i]
			break
		}
	}

	// Remove the target node and refresh while stopped so the prior window
	// records the cleared state.
	n := g.nodeByID(bi.NodeID)
	if n == nil {
		t.Fatalf("node %d missing", bi.NodeID)
	}
	g.deleteNode(n)
	g.updateBeatInfos()
	if g.Playing() {
		stopPlaybackForTest(g)
	}
	g.refreshDrumRow()

	// Re-add the node in the same spot and reconnect the local segment.
	g.SetPlaying(true)
	g.pendingStartRow = row
	re := g.tryAddNode(bi.I, bi.J, model.NodeTypeRegular)
	if p := g.nodeByID(prev.NodeID); p != nil {
		g.addEdge(p, re)
	}
	if nx := g.nodeByID(next.NodeID); nx != nil {
		g.addEdge(re, nx)
	}
	g.pendingStartRow = -1
	g.updateBeatInfos()

	// Simulate playback far enough ahead to freeze history while keeping the
	// target subdivision in the future (next beat).
	g.elapsedBeats = targetAbs + g.grid.MaxDiv()*2
	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}
	if len(g.seqNextIdxs) != len(g.drum.Rows) {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}
	g.nextBeatIdxs[row] = targetAbs
	g.seqNextIdxs[row] = targetAbs
	if len(g.frozenUpToByRow) != len(g.drum.Rows) {
		g.frozenUpToByRow = make([]int, len(g.drum.Rows))
		for i := range g.frozenUpToByRow {
			g.frozenUpToByRow[i] = -1
		}
	}
	g.frozenUpToByRow[row] = targetAbs + g.grid.MaxDiv()

	// Ensure the target cell is within the visible window.
	g.drum.Offset = targetAbs - 1
	if g.drum.Offset < 0 {
		g.drum.Offset = 0
	}
	if g.drum.Offset >= g.nextBeatIdxs[row] {
		t.Fatalf("expected preview window (offset=%d next=%d)", g.drum.Offset, g.nextBeatIdxs[row])
	}

	g.engine.Predictor.Ensure(targetAbs + g.grid.MaxDiv())
	want := g.engine.Predictor.VisibleAt(row, targetAbs)
	if !want {
		t.Fatalf("engine predictor expected true at row=%d abs=%d", row, targetAbs)
	}
	g.refreshDrumRow()

	idx := targetAbs - g.drum.Offset
	if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
		t.Fatalf("target abs=%d idx=%d outside window offset=%d len=%d", targetAbs, idx, g.drum.Offset, len(g.drum.Rows[row].Steps))
	}

	got := g.drum.Rows[row].Steps[idx]
	if got != want {
		t.Fatalf("predictor/view mismatch after re-add: row=%d abs=%d freeze=%d want=%v got=%v offset=%d next=%d", row, targetAbs, g.frozenUpToByRow[row], want, got, g.drum.Offset, g.nextBeatIdxs[row])
	}

}

// Future playback commits that leak past the playhead should be demoted even
// after pausing; otherwise re-added nodes stay masked when the window is
// rebuilt in preview mode.
func TestTimelinePlaybackCommitInFutureDemotedWhenPaused(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetLength(64)

	// Simple square loop on row 0.
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

	g.updateBeatInfos()
	g.refreshDrumRow()

	// Simulate playback state with a future immutable commit.
	g.SetPlaying(true)
	g.nextBeatIdxs = []int{8} // playhead around beat 8
	g.frozenUpToByRow = []int{64} // freeze far ahead
	targetAbs := 20               // lies in the future relative to playhead

	// Seed a playback commit (stale mask) in the future window.
	g.recordTimelineCommitKind(0, targetAbs, false, model.NodeTypeRegular, timeline.CommitKindPlayback)
	g.engine.Predictor.Ensure(targetAbs + 4)
	want := g.engine.Predictor.VisibleAt(0, targetAbs)
	if !want {
		t.Fatalf("predictor expected ON at abs=%d", targetAbs)
	}

	// Pause playback; refresh should demote the future commit and trust the predictor.
	g.SetPlaying(false)
	g.drum.Offset = targetAbs - 2
	g.refreshDrumRow()

	idx := targetAbs - g.drum.Offset
	if idx < 0 || idx >= len(g.drum.Rows[0].Steps) {
		t.Fatalf("target idx %d out of window offset=%d len=%d", idx, g.drum.Offset, len(g.drum.Rows[0].Steps))
	}
	got := g.drum.Rows[0].Steps[idx]
	if got != want {
		v, typ, kind, ok := g.timelineCommittedWithKind(0, targetAbs)
		t.Fatalf("future playback commit not demoted after pause: want=%v got=%v commit=%v typ=%v kind=%v ok=%v next=%v freeze=%v",
			want, got, v, typ, kind, ok, g.nextBeatIdxs, g.frozenUpToByRow)
	}
	// Demotion should preserve the recorded value/type for debugging while
	// relaxing immutability (CommitKindReleased).
	v, typ, kind, ok := g.timelineCommittedWithKind(0, targetAbs)
	if !ok {
		t.Fatalf("expected timeline commit retained after demotion")
	}
	if kind != timeline.CommitKindReleased {
		t.Fatalf("expected future commit demoted to Released; got %v", kind)
	}
	if v {
		t.Fatalf("expected demoted commit to preserve recorded value=false; got true")
	}
	if typ != model.NodeTypeRegular {
		t.Fatalf("expected demoted commit to preserve recorded type regular; got %v", typ)
	}
}

// Repro using the real ./tunkul.json project: during playback, deleting and
// re‑adding a node on row 4 should not leave DrumView masked by an old commit.
// The user observed TIMELINE_TRACE mismatches in /tmp/llm.log for this flow.
func TestImportPlaybackReaddTunkulProject(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)

	if err := g.Import([]byte(tunkulProjectJSON)); err != nil {
		t.Fatalf("import tunkul project: %v", err)
	}
	ensureRowInstrumentsAvailable(t, g)
	g.updateBeatInfos()
	g.refreshDrumRow()

	row := 4 // sample-high-tom row in the imported project
	if row >= len(g.drum.Rows) {
		t.Fatalf("row %d out of range", row)
	}
	originID := g.drum.Rows[row].Origin
	if originID == model.InvalidNodeID {
		t.Fatalf("row %d missing origin", row)
	}
	if row >= len(g.beatInfosByRow) || len(g.beatInfosByRow[row]) == 0 {
		t.Fatalf("row %d missing beat infos", row)
	}
	path := g.beatInfosByRow[row]
	targetAbs := -1
	var bi model.BeatInfo
	for i, info := range path {
		if info.NodeID != model.InvalidNodeID && info.NodeType == model.NodeTypeRegular && info.NodeID != originID {
			targetAbs = i
			bi = info
			break
		}
	}
	if targetAbs < 0 {
		t.Fatalf("no non-origin regular node found for row=%d", row)
	}
	prev := model.BeatInfo{NodeID: model.InvalidNodeID}
	for i := targetAbs - 1; i >= 0; i-- {
		if path[i].NodeID != model.InvalidNodeID {
			prev = path[i]
			break
		}
	}
	nextBi := model.BeatInfo{NodeID: model.InvalidNodeID}
	for i := targetAbs + 1; i < len(path); i++ {
		if path[i].NodeID != model.InvalidNodeID {
			nextBi = path[i]
			break
		}
	}
	driveSchedulerToAbs(t, g, row, targetAbs+g.grid.MaxDiv())
	if len(g.nextBeatIdxs) <= row {
		t.Fatalf("nextBeatIdxs too short: %v", g.nextBeatIdxs)
	}
	t.Logf("row=%d frozen=%v next=%d offset=%d", row, g.frozenUpToByRow, g.nextBeatIdxs[row], g.drum.Offset)

	// Delete and re-add the target node while preserving the local segment.
	n := g.nodeByID(bi.NodeID)
	if n == nil {
		t.Fatalf("node %d missing", bi.NodeID)
	}
	g.deleteNode(n)
	g.updateBeatInfos()

	g.pendingStartRow = row
	re := g.tryAddNode(bi.I, bi.J, model.NodeTypeRegular)
	if p := g.nodeByID(prev.NodeID); p != nil {
		g.addEdge(p, re)
	}
	if nx := g.nodeByID(nextBi.NodeID); nx != nil {
		g.addEdge(re, nx)
	}
	g.pendingStartRow = -1
	g.updateBeatInfos()

	// Advance scheduling with the modified graph to update parity buffers.
	nextIdx := 0
	if row < len(g.nextBeatIdxs) {
		nextIdx = g.nextBeatIdxs[row]
	}
	driveSchedulerToAbs(t, g, row, nextIdx+g.grid.MaxDiv())
	g.engine.Predictor.Ensure(targetAbs + g.grid.MaxDiv())
	// Sweep offsets around the current playhead and assert predictor/view
	// parity for future cells (abs >= nextBeatIdxs[row]) where no immutable
	// playback/import commit exists.
	nextIdx = g.nextBeatIdxs[row]
	// Stop playback before the manual window sweep so parity scans don't
	// treat offset probing as a scheduler vs view mismatch.
	g.SetPlaying(false)
	for off := targetAbs - g.drum.Length; off < nextIdx+g.drum.Length; off++ {
		if off < 0 {
			continue
		}
		g.drum.Offset = off
		g.refreshDrumRow()
		for i := 0; i < g.drum.Length; i++ {
			abs := g.drum.Offset + i
			if abs < nextIdx {
				continue // ignore past; immutable history may differ
			}
			if v, _, kind, ok := g.timelineCommittedWithKind(row, abs); ok && (kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport) {
				// Skip immutable commits; they intentionally lock history.
				_ = v
				continue
			}
			b := g.beatInfoAtRow(row, abs)
			want := g.engine.Predictor.VisibleAt(row, abs)
			got := g.drum.Rows[row].Steps[i]
			if got != want {
				v, typ, kind, ok := g.timelineCommittedWithKind(row, abs)
				t.Fatalf("imported project parity mismatch row=%d abs=%d off=%d want=%v got=%v next=%d freeze=%v commit=%v typ=%v kind=%v ok=%v bi=%v",
					row, abs, g.drum.Offset, want, got, nextIdx, g.frozenUpToByRow, v, typ, kind, ok, b)
			}
		}
	}
}

// Parity-watch repro: inserting a new node along an existing edge, deleting it,
// and re-adding it while playback is running should not produce scheduler vs
// DrumView mismatches even when PARITY_WATCH is active.
func TestImportPlaybackAddRemoveReaddParityWatch(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.parityWatch = parityWatchLog
	g.Layout(1024, 720)

	if err := g.Import([]byte(tunkulProjectJSON)); err != nil {
		t.Fatalf("import tunkul project: %v", err)
	}
	ensureRowInstrumentsAvailable(t, g)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Choose the vertical edge (-36,8) -> (-36,10) on the tom row and insert a
	// node between them at (-36,9). This matches the user flow that triggered
	// a parity panic when PARITY_WATCH=panic in the app.
	pred := g.nodeAt(-36, 8)
	succ := g.nodeAt(-36, 10)
	if pred == nil || succ == nil {
		t.Fatalf("expected nodes at (-36,8) and (-36,10), got pred=%v succ=%v", pred, succ)
	}
	if g.nodeAt(-36, 9) != nil {
		t.Fatalf("unexpected node already at (-36,9)")
	}

	// Seed parity buffers.
	row := 4
	driveSchedulerToAbs(t, g, row, g.grid.MaxDiv()*4)
	// Clear any warm-up mismatches so the test focuses on the add/remove/readd sequence.
	g.ClearParityMismatches()
	g.parityWatch = parityWatchPanic

	insert := g.tryAddNode(-36, 9, model.NodeTypeRegular)
	if insert == nil {
		t.Fatalf("failed to insert node at (-36,9)")
	}

	// Advance scheduling on the modified path.
	driveSchedulerToAbs(t, g, row, g.seqNextIdxs[row]+g.grid.MaxDiv())

	g.deleteNode(insert)

	driveSchedulerToAbs(t, g, row, g.seqNextIdxs[row]+g.grid.MaxDiv())

	readd := g.tryAddNode(-36, 9, model.NodeTypeRegular)
	if readd == nil {
		t.Fatalf("failed to re-add node at (-36,9)")
	}

	driveSchedulerToAbs(t, g, row, g.seqNextIdxs[row]+g.grid.MaxDiv())

	if mism := g.parityRing.snapshot(); len(mism) > 0 {
		t.Fatalf("parity mismatches after add/remove/readd (count=%d): %+v", len(mism), mism)
	}
}

// Regression: deleting and re-adding the same existing node on the imported
// tom row during playback should not leave DrumView masked relative to the
// engine predictor (row=4 at grid=-36,10 in the repro log).
func TestImportPlaybackDeleteReaddSameNodeKeepsPredictorParity(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)
	g.parityWatch = parityWatchLog

	if err := g.Import([]byte(tunkulProjectJSON)); err != nil {
		t.Fatalf("import tunkul project: %v", err)
	}
	ensureRowInstrumentsAvailable(t, g)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.SetBPM(480)

	row := 4
	targetI, targetJ := -36, 10
	target := g.nodeAt(targetI, targetJ)
	if target == nil {
		t.Fatalf("missing target node at (%d,%d)", targetI, targetJ)
	}
	if row >= len(g.beatInfosByRow) || len(g.beatInfosByRow[row]) == 0 {
		t.Fatalf("row %d missing beat infos", row)
	}
	path := g.beatInfosByRow[row]
	targetRel := -1
	for i, bi := range path {
		if bi.I == targetI && bi.J == targetJ {
			targetRel = i
			break
		}
	}
	if targetRel < 0 {
		t.Fatalf("target coord (%d,%d) missing from row=%d path", targetI, targetJ, row)
	}

	// Drive scheduling enough to seed frozen history on the tom row.
	driveSchedulerToAbs(t, g, row, targetRel+g.grid.MaxDiv()*2)
	g.refreshDrumRow()
	if row >= len(g.frozenUpToByRow) || g.frozenUpToByRow[row] < 0 {
		t.Fatalf("row %d never froze: freezes=%v", row, g.frozenUpToByRow)
	}
	if row >= len(g.nextBeatIdxs) {
		t.Fatalf("nextBeatIdxs too short: %v", g.nextBeatIdxs)
	}

	pathLen := len(path)
	next := g.nextBeatIdxs[row]
	if next < 0 {
		next = 0
	}
	targetAbs := next + ((targetRel - (next % pathLen) + pathLen) % pathLen)

	// Delete the existing node and allow the sequencer to settle.
	g.deleteNode(target)
	driveSchedulerToAbs(t, g, row, g.seqNextIdxs[row]+g.grid.MaxDiv())

	// Re-add the same node; stitchEdgesAt should split the bridge.
	re := g.tryAddNode(targetI, targetJ, model.NodeTypeRegular)
	if re == nil {
		t.Fatalf("failed to re-add node at (%d,%d)", targetI, targetJ)
	}
	driveSchedulerToAbs(t, g, row, g.seqNextIdxs[row]+g.grid.MaxDiv())

	// Ensure targetAbs is in the mutable future window.
	next = g.nextBeatIdxs[row]
	if next < 0 {
		next = 0
	}
	for targetAbs < next {
		targetAbs += pathLen
	}
	if bi := g.beatInfoAtRow(row, targetAbs); bi.I != targetI || bi.J != targetJ {
		found := -1
		for i := 0; i < pathLen; i++ {
			abs := next + i
			bi := g.beatInfoAtRow(row, abs)
			if bi.I == targetI && bi.J == targetJ {
				found = abs
				break
			}
		}
		if found < 0 {
			t.Fatalf("could not find re-added node near next=%d row=%d", next, row)
		}
		targetAbs = found
	}

	g.drum.Offset = targetAbs - g.drum.Length/2
	if g.drum.Offset < 0 {
		g.drum.Offset = 0
	}
	g.refreshDrumRow()

	idx := targetAbs - g.drum.Offset
	if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
		t.Fatalf("target abs=%d outside window offset=%d len=%d", targetAbs, g.drum.Offset, len(g.drum.Rows[row].Steps))
	}
	g.engine.Predictor.Ensure(targetAbs + 1)
	want := g.engine.Predictor.VisibleAt(row, targetAbs)
	if !want {
		t.Fatalf("predictor expected ON at row=%d abs=%d (freeze=%v next=%d)", row, targetAbs, g.frozenUpToByRow, next)
	}
	got := g.drum.Rows[row].Steps[idx]
	if got != want {
		v, typ, kind, ok := g.timelineCommittedWithKind(row, targetAbs)
		t.Fatalf("predictor/view mismatch after delete+readd row=%d abs=%d idx=%d want=%v got=%v freeze=%v next=%v commit=%v typ=%v kind=%v ok=%v",
			row, targetAbs, idx, want, got, g.frozenUpToByRow, g.nextBeatIdxs, v, typ, kind, ok)
	}
	if audible := g.engine.Predictor.AudibleAt(row, targetAbs); audible != want {
		t.Fatalf("predictor audible mismatch row=%d abs=%d audible=%v visible=%v", row, targetAbs, audible, want)
	}
}

// Regression: after deleting and re-adding the same node, the sequencer should
// resume scheduling that node on the row (audio should not silently disappear).
func TestImportPlaybackDeleteReaddReschedulesNode(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)

	if err := g.Import([]byte(tunkulProjectJSON)); err != nil {
		t.Fatalf("import tunkul project: %v", err)
	}
	ensureRowInstrumentsAvailable(t, g)
	g.updateBeatInfos()
	g.refreshDrumRow()

	row := 4
	targetI, targetJ := -36, 10
	target := g.nodeAt(targetI, targetJ)
	if target == nil {
		t.Fatalf("missing target node at (%d,%d)", targetI, targetJ)
	}
	if row >= len(g.drum.Rows) {
		t.Fatalf("row %d out of range", row)
	}
	clearMuteSolo(t, g.drum)

	var wantID atomic.Int64
	var saw atomic.Bool
	wantID.Store(int64(target.ID))
	g.scheduleHook = func(r, idx int) {
		if r != row {
			return
		}
		bi := g.beatInfoAtRow(r, idx)
		if bi.NodeType != model.NodeTypeRegular {
			return
		}
		if int64(bi.NodeID) == wantID.Load() {
			saw.Store(true)
		}
	}
	g.drum.SetBPM(480)

	path := g.beatInfosByRow[row]
	targetRel := -1
	for i, bi := range path {
		if bi.I == targetI && bi.J == targetJ {
			targetRel = i
			break
		}
	}
	if targetRel < 0 {
		t.Fatalf("target coord (%d,%d) missing from row=%d path", targetI, targetJ, row)
	}

	maxAbs := targetRel + len(path)*2
	driveSchedulerToAbs(t, g, row, maxAbs)
	if !saw.Load() {
		t.Fatalf("did not schedule target node before delete: id=%d row=%d", target.ID, row)
	}

	g.deleteNode(target)

	re := g.tryAddNode(targetI, targetJ, model.NodeTypeRegular)
	if re == nil {
		t.Fatalf("failed to re-add node at (%d,%d)", targetI, targetJ)
	}
	wantID.Store(int64(re.ID))
	saw.Store(false)

	maxAbs = g.seqNextIdxs[row] + len(path)*2
	driveSchedulerToAbs(t, g, row, maxAbs)
	if !saw.Load() {
		t.Fatalf("re-added node never rescheduled: old=%d new=%d row=%d", target.ID, re.ID, row)
	}
}

// Stale sequencer highlights (queued before a path change) should not create
// immutable commits that mask the predictor after a delete+re-add flow.
func TestImportPlaybackStaleHighlightAfterReaddMasksPredictor(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)

	if err := g.Import([]byte(tunkulProjectJSON)); err != nil {
		t.Fatalf("import tunkul project: %v", err)
	}
	ensureRowInstrumentsAvailable(t, g)
	g.updateBeatInfos()
	g.refreshDrumRow()

	row := 4
	target := g.nodeAt(-36, 10)
	if target == nil {
		t.Fatalf("missing target node at (-36,10)")
	}

	const horizon = 200
	g.engine.Predictor.Ensure(horizon + 1)
	oldInfo := make(map[int]model.BeatInfo, horizon+1)
	oldAudible := make(map[int]bool, horizon+1)
	for abs := 0; abs <= horizon; abs++ {
		oldInfo[abs] = g.beatInfoAtRow(row, abs)
		oldAudible[abs] = g.engine.Predictor.AudibleAt(row, abs)
	}

	// Delete the node to force a path change.
	g.deleteNode(target)
	g.engine.Predictor.Ensure(horizon + 1)

	// Find an abs where the old path was audible but the new predictor says off.
	abs := -1
	var info model.BeatInfo
	for i := 0; i <= horizon; i++ {
		if !oldAudible[i] {
			continue
		}
		if oldInfo[i].NodeType != model.NodeTypeRegular {
			continue
		}
		if g.engine.Predictor.AudibleAt(row, i) {
			continue
		}
		abs = i
		info = oldInfo[i]
		break
	}
	if abs < 0 {
		t.Fatalf("no candidate abs found where old audible != new audible (row=%d)", row)
	}

	// Simulate the sequencer racing ahead of the UI playhead, then apply a stale highlight.
	g.SetPlaying(true)
	g.elapsedBeats = abs + 10
	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}
	if len(g.seqNextIdxs) != len(g.drum.Rows) {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}
	if abs > 0 {
		g.nextBeatIdxs[row] = abs - 1
	}
	g.seqNextIdxs[row] = abs + 5

	g.applySequencerHighlight(row, abs, info)

	g.drum.Offset = abs - 1
	if g.drum.Offset < 0 {
		g.drum.Offset = 0
	}
	g.refreshDrumRow()

	idx := abs - g.drum.Offset
	if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
		t.Fatalf("abs %d not in window offset=%d len=%d", abs, g.drum.Offset, len(g.drum.Rows[row].Steps))
	}
	want := g.engine.Predictor.AudibleAt(row, abs)
	got := g.drum.Rows[row].Steps[idx]
	if got != want {
		t.Fatalf("stale highlight masked predictor: row=%d abs=%d want=%v got=%v next=%v seqNext=%v freeze=%v info=%v",
			row, abs, want, got, g.nextBeatIdxs, g.seqNextIdxs, g.frozenUpToByRow, info)
	}
}

// Stale sequencer highlights that arrive for the current beat (idx == next-1)
// after a path change should be ignored so they don't freeze incorrect history.
func TestImportPlaybackStaleHighlightCurrentBeatAfterReadd(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)

	if err := g.Import([]byte(tunkulProjectJSON)); err != nil {
		t.Fatalf("import tunkul project: %v", err)
	}
	ensureRowInstrumentsAvailable(t, g)
	g.updateBeatInfos()
	g.refreshDrumRow()

	row := 4
	target := g.nodeAt(-36, 10)
	if target == nil {
		t.Fatalf("missing target node at (-36,10)")
	}

	const horizon = 200
	g.engine.Predictor.Ensure(horizon + 1)
	oldInfo := make(map[int]model.BeatInfo, horizon+1)
	oldAudible := make(map[int]bool, horizon+1)
	for abs := 0; abs <= horizon; abs++ {
		oldInfo[abs] = g.beatInfoAtRow(row, abs)
		oldAudible[abs] = g.engine.Predictor.AudibleAt(row, abs)
	}

	// Delete the node to force a path change.
	g.deleteNode(target)
	g.engine.Predictor.Ensure(horizon + 1)

	// Find an abs where the old path was audible but the new predictor says off.
	abs := -1
	var info model.BeatInfo
	for i := 0; i <= horizon; i++ {
		if !oldAudible[i] {
			continue
		}
		if oldInfo[i].NodeType != model.NodeTypeRegular {
			continue
		}
		if g.engine.Predictor.AudibleAt(row, i) {
			continue
		}
		abs = i
		info = oldInfo[i]
		break
	}
	if abs < 0 {
		t.Fatalf("no candidate abs found where old audible != new audible (row=%d)", row)
	}

	// Simulate the sequencer delivering a stale highlight for the *current* beat.
	g.SetPlaying(true)
	g.elapsedBeats = abs
	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}
	if len(g.seqNextIdxs) != len(g.drum.Rows) {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}
	g.nextBeatIdxs[row] = abs + 1
	g.seqNextIdxs[row] = abs + 1

	g.applySequencerHighlight(row, abs, info)

	g.drum.Offset = abs - 1
	if g.drum.Offset < 0 {
		g.drum.Offset = 0
	}
	g.refreshDrumRow()

	idx := abs - g.drum.Offset
	if idx < 0 || idx >= len(g.drum.Rows[row].Steps) {
		t.Fatalf("abs %d not in window offset=%d len=%d", abs, g.drum.Offset, len(g.drum.Rows[row].Steps))
	}
	want := g.engine.Predictor.AudibleAt(row, abs)
	got := g.drum.Rows[row].Steps[idx]
	if got != want {
		t.Fatalf("stale current-beat highlight masked predictor: row=%d abs=%d want=%v got=%v next=%v seqNext=%v info=%v",
			row, abs, want, got, g.nextBeatIdxs, g.seqNextIdxs, info)
	}
}

const tunkulProjectJSON = `{
  "version": 1,
  "subdiv": 8,
  "bpm": 120,
  "instruments": [
    {
      "name": "Sample-kick-9-wonder-lop-k1",
      "id": "sample-kick-9-wonder-lop-k1",
      "kind": "sample",
      "volume": 1,
      "origin": 0,
      "color": "#C87850FF"
    },
    {
      "name": "Sample-snare",
      "id": "sample-snare",
      "kind": "sample",
      "volume": 0.67,
      "origin": 3,
      "color": "#CE3FFFFF"
    },
    {
      "name": "Sample-house-closed-hi-hat",
      "id": "sample-house-closed-hi-hat",
      "kind": "sample",
      "volume": 1,
      "origin": 7,
      "color": "#FFF610FF"
    },
    {
      "name": "Sample-house-open-hi-hat",
      "id": "sample-house-open-hi-hat",
      "kind": "sample",
      "volume": 0.76,
      "origin": 15,
      "color": "#27FF5DFF"
    },
    {
      "name": "Sample-high-tom-9-wonder",
      "id": "sample-high-tom-9-wonder",
      "kind": "sample",
      "volume": 0.78,
      "origin": 17,
      "color": "#50C8C8FF"
    },
    {
      "name": "Sample-house-closed-hi-hat",
      "id": "sample-house-closed-hi-hat",
      "kind": "sample",
      "volume": 0.23,
      "origin": 28,
      "color": "#FF5425FF"
    }
  ],
  "nodes": [
    {
      "id": 0,
      "i": -16,
      "j": -8,
      "type": "regular",
      "inputs": [
        26
      ],
      "outputs": [
        23
      ]
    },
    {
      "id": 1,
      "i": -8,
      "j": 0,
      "type": "regular",
      "inputs": [
        11
      ],
      "outputs": [
        2
      ]
    },
    {
      "id": 2,
      "i": -16,
      "j": 0,
      "type": "regular",
      "inputs": [
        1
      ],
      "outputs": [
        26
      ],
      "volume": 1.5000000000000004
    },
    {
      "id": 3,
      "i": 0,
      "j": -16,
      "type": "regular",
      "inputs": [
        24
      ],
      "outputs": [
        27
      ],
      "volume": 1.4999999999999996,
      "pitch": -4,
      "duration": 1.5000000000000004
    },
    {
      "id": 4,
      "i": 16,
      "j": -16,
      "type": "regular",
      "inputs": [
        25
      ],
      "outputs": [
        5
      ],
      "volume": 2,
      "pitch": 2,
      "duration": 0.8
    },
    {
      "id": 5,
      "i": 16,
      "j": 0,
      "type": "regular",
      "inputs": [
        4
      ],
      "outputs": [
        6
      ],
      "volume": 2.000000000000001,
      "pitch": 2,
      "duration": 0.8
    },
    {
      "id": 6,
      "i": 4,
      "j": 0,
      "type": "regular",
      "inputs": [
        5
      ],
      "outputs": [
        12
      ],
      "volume": 2,
      "duration": 1.3000000000000003
    },
    {
      "id": 7,
      "i": 8,
      "j": 8,
      "type": "regular",
      "inputs": [
        14
      ],
      "outputs": [
        30
      ]
    },
    {
      "id": 8,
      "i": 12,
      "j": 12,
      "type": "regular",
      "inputs": [
        13
      ],
      "outputs": [
        14
      ]
    },
    {
      "id": 9,
      "i": -8,
      "j": 8,
      "type": "regular",
      "inputs": [
        15
      ],
      "outputs": [
        16
      ]
    },
    {
      "id": 10,
      "i": -24,
      "j": 24,
      "type": "regular",
      "inputs": [
        16
      ],
      "outputs": [
        32
      ]
    },
    {
      "id": 11,
      "i": -8,
      "j": -8,
      "type": "regular",
      "inputs": [
        22
      ],
      "outputs": [
        1
      ],
      "volume": 2,
      "pitch": 15,
      "duration": 2.450000000000001
    },
    {
      "id": 12,
      "i": 0,
      "j": 0,
      "type": "regular",
      "inputs": [
        6
      ],
      "outputs": [
        24
      ],
      "volume": 2,
      "pitch": 4,
      "duration": 2.000000000000001
    },
    {
      "id": 13,
      "i": 12,
      "j": 8,
      "type": "regular",
      "inputs": [
        30
      ],
      "outputs": [
        8
      ],
      "volume": 2.000000000000001,
      "duration": 0.8
    },
    {
      "id": 14,
      "i": 8,
      "j": 12,
      "type": "regular",
      "inputs": [
        8
      ],
      "outputs": [
        7
      ]
    },
    {
      "id": 15,
      "i": -24,
      "j": 8,
      "type": "regular",
      "inputs": [
        33
      ],
      "outputs": [
        9
      ]
    },
    {
      "id": 16,
      "i": -8,
      "j": 24,
      "type": "regular",
      "inputs": [
        9
      ],
      "outputs": [
        10
      ]
    },
    {
      "id": 17,
      "i": -32,
      "j": 8,
      "type": "regular",
      "inputs": [
        21
      ],
      "outputs": [
        20
      ],
      "volume": 2,
      "duration": 1.4500000000000002
    },
    {
      "id": 18,
      "i": -32,
      "j": 12,
      "type": "regular",
      "inputs": [
        19
      ],
      "outputs": [
        21
      ],
      "skip_every": 2,
      "logic_kind": "skip_every_n",
      "logic_n": 2
    },
    {
      "id": 19,
      "i": -36,
      "j": 12,
      "type": "regular",
      "inputs": [
        31
      ],
      "outputs": [
        18
      ],
      "volume": 1.5000000000000004,
      "pitch": -2,
      "duration": 1.5000000000000004,
      "skip_every": 2,
      "logic_kind": "skip_every_n",
      "logic_n": 2
    },
    {
      "id": 20,
      "i": -36,
      "j": 8,
      "type": "regular",
      "inputs": [
        17
      ],
      "outputs": [
        31
      ],
      "volume": 1.5000000000000004,
      "pitch": -2,
      "duration": 1.8000000000000007
    },
    {
      "id": 21,
      "i": -32,
      "j": 10,
      "type": "regular",
      "inputs": [
        18
      ],
      "outputs": [
        17
      ],
      "pitch": -6,
      "duration": 0.7000000000000001,
      "skip_every": 2,
      "logic_kind": "skip_every_n",
      "logic_n": 2
    },
    {
      "id": 22,
      "i": -10,
      "j": -8,
      "type": "regular",
      "inputs": [
        23
      ],
      "outputs": [
        11
      ],
      "volume": 1.5000000000000004
    },
    {
      "id": 23,
      "i": -12,
      "j": -8,
      "type": "regular",
      "inputs": [
        0
      ],
      "outputs": [
        22
      ],
      "volume": 1.5000000000000004,
      "pitch": 6,
      "duration": 0.8
    },
    {
      "id": 24,
      "i": 0,
      "j": -2,
      "type": "regular",
      "inputs": [
        12
      ],
      "outputs": [
        3
      ],
      "volume": 2,
      "pitch": 4
    },
    {
      "id": 25,
      "i": 14,
      "j": -16,
      "type": "regular",
      "inputs": [
        27
      ],
      "outputs": [
        4
      ],
      "volume": 1.5000000000000004,
      "pitch": -4,
      "duration": 1.5000000000000004
    },
    {
      "id": 26,
      "i": -16,
      "j": -2,
      "type": "regular",
      "inputs": [
        2
      ],
      "outputs": [
        0
      ],
      "volume": 1.5000000000000004,
      "pitch": -2,
      "duration": 0.8
    },
    {
      "id": 27,
      "i": 2,
      "j": -16,
      "type": "regular",
      "inputs": [
        3
      ],
      "outputs": [
        25
      ],
      "volume": 1.5000000000000004,
      "pitch": -3,
      "duration": 1.2000000000000002
    },
    {
      "id": 28,
      "i": -33,
      "j": -1,
      "type": "regular",
      "inputs": [
        29
      ],
      "outputs": [
        29
      ],
      "groove_kind": "rush",
      "groove_pct": 1.3877787807814457e-17
    },
    {
      "id": 29,
      "i": -31,
      "j": -1,
      "type": "regular",
      "inputs": [
        28
      ],
      "outputs": [
        28
      ],
      "skip_every": 4,
      "logic_kind": "skip_every_n",
      "logic_n": 4,
      "groove_kind": "delay",
      "groove_pct": 0.10000000000000002
    },
    {
      "id": 30,
      "i": 10,
      "j": 8,
      "type": "regular",
      "inputs": [
        7
      ],
      "outputs": [
        13
      ]
    },
    {
      "id": 31,
      "i": -36,
      "j": 10,
      "type": "regular",
      "inputs": [
        20
      ],
      "outputs": [
        19
      ],
      "volume": 2.000000000000001
    },
    {
      "id": 32,
      "i": -24,
      "j": 19,
      "type": "regular",
      "inputs": [
        10
      ],
      "outputs": [
        33
      ]
    },
    {
      "id": 33,
      "i": -24,
      "j": 10,
      "type": "regular",
      "inputs": [
        32
      ],
      "outputs": [
        15
      ]
    }
  ]
}`
