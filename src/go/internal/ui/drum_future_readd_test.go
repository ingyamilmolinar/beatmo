package ui

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

func advancePlayback(g *Game, dur time.Duration) {
	if g == nil {
		return
	}
	if !g.Playing() {
		advanceFrames(g, 5)
		return
	}
	div := g.grid.MaxDiv()
	if div <= 0 {
		div = 1
	}
	bpm := g.AppliedBPM()
	if bpm <= 0 {
		bpm = g.drum.BPM()
	}
	if bpm <= 0 {
		bpm = 120
	}
	beats := dur.Seconds() * float64(bpm) / 60.0
	steps := int(math.Ceil(beats * float64(div)))
	if steps < 1 {
		steps = 1
	}
	start := g.elapsedBeats
	for abs := start; abs <= start+steps; abs++ {
		setPlayStartForAbs(g, abs)
		_ = g.Update()
	}
}

func waitLeadBelow(g *Game, row int, threshold int, timeout time.Duration) error {
	maxIters := int(math.Ceil(timeout.Seconds()*50)) + 1
	if maxIters < 1 {
		maxIters = 1
	}
	for i := 0; i < maxIters; i++ {
		if row < len(g.nextBeatIdxs) {
			lead := g.nextBeatIdxs[row] - g.drum.Offset
			if lead <= threshold {
				return nil
			}
		}
		advancePlayback(g, 10*time.Millisecond)
	}
	return errors.New("lead did not fall below threshold")
}

func futureRelForRow(g *Game, row int) int {
	next := g.drum.Offset
	if row >= 0 && row < len(g.nextBeatIdxs) {
		next = g.nextBeatIdxs[row]
	}
	start := next - g.drum.Offset
	if start < 0 {
		start = 0
	}
	steps := g.drum.Rows[row].Steps
	for i := start; i < len(steps); i++ {
		if steps[i] {
			return i
		}
	}
	return -1
}

func TestUpdateBeatInfos_ReaddAdvancesNextBeat(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)
	g.drum.SetFollow(true)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	d := g.tryAddNode(3, 0, model.NodeTypeRegular)
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

	if len(g.nextBeatIdxs) == 0 {
		t.Fatalf("expected nextBeatIdxs initialized")
	}
	before := g.nextBeatIdxs[0]

	g.deleteNode(b)
	g.updateBeatInfos()
	if len(g.nextBeatIdxs) == 0 {
		t.Fatalf("nextBeatIdxs lost after delete")
	}
	if got := g.nextBeatIdxs[0]; got <= before {
		t.Fatalf("delete did not advance next beat: before=%d after=%d", before, got)
	}

	newB := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.deleteEdge(a, d)
	g.addEdge(a, newB)
	g.addEdge(newB, c)
	g.updateBeatInfos()
	if len(g.nextBeatIdxs) == 0 {
		t.Fatalf("nextBeatIdxs empty after re-add")
	}
	if g.nextBeatIdxs[0] != before {
		t.Fatalf("expected next beat to restore after re-add, got=%d want=%d", g.nextBeatIdxs[0], before)
	}
}

func TestDrumView_ReaddTwiceBeforeTriggerUpdatesFuture(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetFollow(false)

	start := 48
	step := g.grid.MaxDiv()
	nodes := make([]*uiNode, 4)
	for i := 0; i < 4; i++ {
		nodes[i] = g.tryAddNode(start+i*step, 0, model.NodeTypeRegular)
		if i > 0 {
			g.addEdge(nodes[i-1], nodes[i])
		}
	}
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.drum.Rows[0].Origin = nodes[0].ID
	g.drum.Rows[0].Node = nodes[0]
	g.drum.SetLength(128)
	g.updateBeatInfos()

	dst := ebiten.NewImage(640, 240)
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)

	g.drum.SetBPM(120)
	advancePlayback(g, 100*time.Millisecond)
	pressPlay(t, g.drum)
	advancePlayback(g, 250*time.Millisecond)

	targetRow := 0

	deleteNode := func() {
		if node := g.nodeAt(start+2*step, 0); node != nil {
			g.deleteNode(node)
			g.updateBeatInfos()
		}
	}
	readdNode := func() {
		left := g.nodeAt(start+step, 0)
		right := g.nodeAt(start+3*step, 0)
		if left != nil && right != nil {
			g.deleteEdge(left, right)
		}
		re := g.tryAddNode(start+2*step, 0, model.NodeTypeRegular)
		if left != nil {
			g.addEdge(left, re)
		}
		if right != nil {
			g.addEdge(re, right)
		}
		g.updateBeatInfos()
	}

	g.refreshDrumRow()
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	targetNode := g.nodeAt(start+2*step, 0)
	if targetNode == nil {
		t.Fatalf("target node missing before delete")
	}
	targetAbs := -1
	for i := g.drum.Offset; i < g.drum.Offset+g.drum.Length; i++ {
		if g.beatInfoAtRow(targetRow, i).NodeID == targetNode.ID {
			targetAbs = i
			break
		}
	}
	if targetAbs < 0 {
		t.Fatalf("could not locate target node before delete")
	}
	targetRel := targetAbs - g.drum.Offset

	deleteNode()
	g.refreshDrumRow()
	t.Logf("targetRel=%d steps around=%v", targetRel, g.drum.Rows[targetRow].Steps[targetRel:targetRel+4])
	if targetRel >= 0 && targetRel < len(g.drum.Rows[targetRow].Steps) && g.drum.Rows[targetRow].Steps[targetRel] {
		t.Fatalf("future lamp stayed lit after delete (rel=%d offset=%d)", targetRel, g.drum.Offset)
	}

	readdNode()

	maxIters := int(math.Ceil(0.5/0.01)) + 1
	if maxIters < 1 {
		maxIters = 1
	}
	found := false
	for i := 0; i < maxIters; i++ {
		g.refreshDrumRow()
		if targetRel < len(g.drum.Rows[targetRow].Steps) && g.drum.Rows[targetRow].Steps[targetRel] {
			found = true
			break
		}
		advancePlayback(g, 10*time.Millisecond)
	}
	if !found {
		t.Fatalf("re-add failed to relight future cell (rel=%d offset=%d next=%v)", targetRel, g.drum.Offset, g.nextBeatIdxs)
	}
}

func TestDrumView_ReaddDuringPlaybackLeavesFutureStale(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(960, 600)
	g.drum.SetFollow(false)

	// Build a square loop so deleting one corner fully breaks the circuit
	// without the auto-reconnect logic restoring a straight edge.
	coords := []struct{ i, j int }{
		{0, 0}, {4, 0}, {4, 4}, {0, 4},
	}
	nodes := make([]*uiNode, len(coords))
	for idx, c := range coords {
		nodes[idx] = g.tryAddNode(c.i, c.j, model.NodeTypeRegular)
	}
	for idx := range nodes {
		next := (idx + 1) % len(nodes)
		g.addEdge(nodes[idx], nodes[next])
	}
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.drum.Rows[0].Origin = nodes[0].ID
	g.drum.Rows[0].Node = nodes[0]
	g.drum.SetLength(96)
	g.updateBeatInfos()

	dst := ebiten.NewImage(960, 240)
	g.refreshDrumRow()
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)

	g.drum.SetBPM(120)
	advancePlayback(g, 120*time.Millisecond)
	pressPlay(t, g.drum)
	advancePlayback(g, 260*time.Millisecond)

	// Operate on the corner at coords[1]; scan ahead so the edit occurs well
	// into the future window during playback.
	targetNode := nodes[1]
	targetNodeID := targetNode.ID
	if len(g.nextBeatIdxs) == 0 {
		t.Fatalf("nextBeatIdxs missing before playback edits")
	}
	targetAbs := -1
	startScan := g.nextBeatIdxs[0]
	if startScan < g.drum.Offset {
		startScan = g.drum.Offset
	}
	for abs := startScan; abs < g.drum.Offset+g.drum.Length; abs++ {
		if g.beatInfoAtRow(0, abs).NodeID == targetNodeID {
			targetAbs = abs
			break
		}
	}
	if targetAbs < 0 {
		t.Fatalf("failed to locate future beat for node=%d (startScan=%d offset=%d len=%d next=%v)", targetNodeID, startScan, g.drum.Offset, g.drum.Length, g.nextBeatIdxs)
	}
	targetRel := targetAbs - g.drum.Offset
	if targetRel < 0 || targetRel >= len(g.drum.Rows[0].Steps) {
		t.Fatalf("target rel out of range rel=%d len=%d offset=%d", targetRel, len(g.drum.Rows[0].Steps), g.drum.Offset)
	}
	if n := g.nodeByID(targetNodeID); n != nil {
		targetNode = n
	} else {
		t.Fatalf("target node %#v missing from UI registry", targetNodeID)
	}

	// Capture predecessor/successor so we can stitch the path back together.
	predID := nodes[0].ID
	succID := nodes[2].ID
	if predID == model.InvalidNodeID || succID == model.InvalidNodeID {
		t.Fatalf("unexpected graph topology for target node (pred=%d succ=%d)", predID, succID)
	}

	// Delete the target node via the UI queue while playback runs.
	g.enqueueUI(func() {
		if n := g.nodeByID(targetNodeID); n != nil {
			g.deleteNode(n)
		}
	})
	if err := g.Update(); err != nil {
		t.Fatalf("unexpected update error after delete: %v", err)
	}
	if targetRel < len(g.drum.Rows[0].Steps) && g.drum.Rows[0].Steps[targetRel] {
		t.Fatalf("future step remained active after delete rel=%d", targetRel)
	}

	// Simulate the stale-freeze state we observe in the regression: the guard,
	// max, and preserved history extend into the future beat even before we
	// re-add the node.
	if len(g.frozenUpToByRow) != len(g.drum.Rows) {
		buf := make([]int, len(g.drum.Rows))
		for i := range buf {
			buf[i] = -1
		}
		copy(buf, g.frozenUpToByRow)
		g.frozenUpToByRow = buf
	}
	g.frozenUpToByRow[0] = targetAbs
	info := g.beatInfoAtRow(0, targetAbs)
	g.recordTimelineCommitKind(0, targetAbs, false, info.NodeType, timeline.CommitKindSeeded)

	// Re-add the node at the same coordinates and stitch the edges, again in
	// the UI queue to mirror runtime behaviour.
	g.enqueueUI(func() {
		if g.nodeAt(targetNode.I, targetNode.J) != nil {
			return
		}
		re := g.tryAddNode(targetNode.I, targetNode.J, model.NodeTypeRegular)
		if pred := g.nodeByID(predID); pred != nil {
			g.addEdge(pred, re)
		}
		if succ := g.nodeByID(succID); succ != nil {
			g.addEdge(re, succ)
		}
	})
	if err := g.Update(); err != nil {
		t.Fatalf("unexpected update error after re-add: %v", err)
	}
	state := g.dumpRowState(0)
	rel := targetAbs - state.Timeline.Offset
	var mask bool
	if rel >= 0 && rel < len(state.Timeline.PastMask) {
		mask = state.Timeline.PastMask[rel]
	}
	t.Logf("state after re-add: frozen=%d timelineMask=%v rel=%d",
		state.FrozenUpTo, mask, rel)

	if targetRel >= len(g.drum.Rows[0].Steps) {
		t.Fatalf("target rel out of range after re-add: rel=%d len=%d", targetRel, len(g.drum.Rows[0].Steps))
	}
	if !g.drum.Rows[0].Steps[targetRel] {
		t.Fatalf("future step remained stale after re-add (abs=%d rel=%d)", targetAbs, targetRel)
	}
	if mask {
		t.Fatalf("future timeline entry persisted at abs=%d", targetAbs)
	}
}
