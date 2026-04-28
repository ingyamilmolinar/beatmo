package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Regression: after a play → stop → play cycle, the second playback must
// rebuild the drum row slate just like the first — the timeline must not
// stay all-grey, and highlights must keep following the playhead.
//
// Reported manually as: "after running the circuit for a while, and then
// clicking stop, the timeline shows all grey and with no highlight following
// the current drum view alongside the timeline (as it was working during the
// first playback)."
func TestStopReplayKeepsSlateAndHighlights(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)
	g.drum.SetFollow(true)
	g.drum.SetLength(64)

	// Build a 4-node loop on row 0.
	const loopLen = 4
	nodes := make([]*uiNode, loopLen)
	for i := 0; i < loopLen; i++ {
		nodes[i] = g.tryAddNode(i, 0, model.NodeTypeRegular)
		if i > 0 {
			g.addEdge(nodes[i-1], nodes[i])
		}
	}
	g.addEdge(nodes[loopLen-1], nodes[0])
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.drum.Rows[0].Origin = nodes[0].ID
	g.drum.Rows[0].Node = nodes[0]

	g.drum.SetBPM(120)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Track highlight callback invocations so we can assert the highlight
	// cursor advances during each play burst.
	type hlEvent struct{ row, idx int }
	var firstHL, secondHL []hlEvent
	captureHL := func(target *[]hlEvent) {
		g.highlightHook = func(row, idx int) { *target = append(*target, hlEvent{row, idx}) }
	}

	// ── First playback ───────────────────────────────────────────────────
	captureHL(&firstHL)
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update during first play: %v", err)
	}
	if !g.Playing() {
		t.Fatalf("expected playing after first play")
	}
	advancePlaybackByAbs(g, g.grid.MaxDiv()*32)
	g.refreshDrumRow()

	firstActive := countActiveSteps(g, 0)
	if firstActive == 0 {
		t.Fatalf("first playback: row 0 has no active cells (slate empty); steps=%v",
			g.drum.Rows[0].Steps)
	}
	if len(firstHL) == 0 {
		t.Fatalf("first playback: no highlight callbacks fired (cursor not following)")
	}

	// ── Stop ─────────────────────────────────────────────────────────────
	pressStop(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on stop: %v", err)
	}
	if g.Playing() || g.Paused() {
		t.Fatalf("expected stopped after stop")
	}
	// After Stop: while not playing, refreshDrumRow may be skipped (only fires on
	// dirty/playing). Force a refresh so the slate reflects the post-stop state
	// the user is staring at — it MUST show the predictor's pattern, not all-grey.
	g.refreshDrumRow()
	postStopActive := countActiveSteps(g, 0)
	if postStopActive == 0 {
		t.Fatalf("after stop: row 0 slate is all-grey (Steps all false). "+
			"first playback had %d active. The user sees this static state.",
			firstActive)
	}

	// ── Second playback ──────────────────────────────────────────────────
	captureHL(&secondHL)
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update during second play: %v", err)
	}
	if !g.Playing() {
		t.Fatalf("expected playing after second play")
	}
	advancePlaybackByAbs(g, g.grid.MaxDiv()*32)
	g.refreshDrumRow()

	secondActive := countActiveSteps(g, 0)
	if secondActive == 0 {
		t.Fatalf("second playback: row 0 slate is all-grey (Steps all false); "+
			"first had %d active. steps=%v cellTypes=%v",
			firstActive, g.drum.Rows[0].Steps, g.drum.Rows[0].CellTypes)
	}
	if len(secondHL) == 0 {
		t.Fatalf("second playback: highlight cursor stopped following the playhead "+
			"(no highlightHook callbacks). first had %d events", len(firstHL))
	}
	// The two playbacks should produce roughly the same amount of active
	// cells; allow some tolerance for window/offset drift.
	if secondActive < firstActive/2 {
		t.Fatalf("second playback active cells %d much lower than first %d "+
			"(expected ~equal; this means the predictor/timeline didn't recover "+
			"after stop)", secondActive, firstActive)
	}
}

func countActiveSteps(g *Game, row int) int {
	if row < 0 || row >= len(g.drum.Rows) {
		return 0
	}
	n := 0
	for _, v := range g.drum.Rows[row].Steps {
		if v {
			n++
		}
	}
	return n
}

// TestStopReplayAfterPathEditsKeepsSlate covers the most likely path the user
// hit: they played the circuit "for a while", *edited it during playback*
// (which sets pathChangeBeatByRow[row] = elapsedBeats — a high abs index),
// then pressed Stop and Play again. Without resetting pathChangeBeatByRow,
// the safety-net loop in refreshDrumRow (game_refresh_drum_row.go ≈ 207-228)
// would skip writing released commits for any abs < pathChangeBeat, which
// after stop+replay translates to "every freshly-played cell is below the
// stale path-change watermark, so no released commit is ever written."
// On its own that's bookkeeping, but combined with stale lastTriggeredByRow
// it can drop the highlight-fallback path in highlightVisual and leave the
// timeline empty.
func TestStopReplayAfterPathEditsKeepsSlate(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)
	g.drum.SetFollow(true)
	g.drum.SetLength(64)

	const loopLen = 4
	nodes := make([]*uiNode, loopLen)
	for i := 0; i < loopLen; i++ {
		nodes[i] = g.tryAddNode(i, 0, model.NodeTypeRegular)
		if i > 0 {
			g.addEdge(nodes[i-1], nodes[i])
		}
	}
	g.addEdge(nodes[loopLen-1], nodes[0])
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.drum.Rows[0].Origin = nodes[0].ID
	g.drum.Rows[0].Node = nodes[0]
	g.drum.SetBPM(120)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Play for a while, then mutate the circuit mid-playback so the
	// pathChangeBeatByRow watermark gets set to a non-zero value.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update during play: %v", err)
	}
	advancePlaybackByAbs(g, g.grid.MaxDiv()*16)
	// Toggle a node's logic to force a path-shape change at high elapsedBeats.
	if node, ok := g.graph.GetNodeByID(nodes[2].ID); ok {
		node.Type = model.NodeTypeMute
		g.graph.Nodes[nodes[2].ID] = node
		g.cacheNode(nodes[2].ID)
		g.notifyPredictorNode(nodes[2].ID)
		g.updateBeatInfos()
	}
	advancePlaybackByAbs(g, g.grid.MaxDiv()*8)
	// Restore type so the second-play prediction is identical to the first.
	if node, ok := g.graph.GetNodeByID(nodes[2].ID); ok {
		node.Type = model.NodeTypeRegular
		g.graph.Nodes[nodes[2].ID] = node
		g.cacheNode(nodes[2].ID)
		g.notifyPredictorNode(nodes[2].ID)
		g.updateBeatInfos()
	}
	advancePlaybackByAbs(g, g.grid.MaxDiv()*8)
	g.refreshDrumRow()
	firstActive := countActiveSteps(g, 0)
	if firstActive == 0 {
		t.Fatalf("first playback after edits: row 0 has no active cells")
	}
	stalePathChangeBeat := -1
	if len(g.pathChangeBeatByRow) > 0 {
		stalePathChangeBeat = g.pathChangeBeatByRow[0]
	}

	// Stop
	pressStop(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on stop: %v", err)
	}
	if g.Playing() || g.Paused() {
		t.Fatalf("expected stopped after stop")
	}
	// pathChangeBeatByRow MUST be reset on Stop. If it persists at the high
	// elapsedBeats value from mid-play, the safety-net commit loop will skip
	// every freshly-played cell on replay (because j < stalePathChangeBeat
	// for low j), corrupting timeline reconciliation.
	if len(g.pathChangeBeatByRow) > 0 && g.pathChangeBeatByRow[0] > 0 {
		t.Fatalf("Stop did not reset pathChangeBeatByRow; row 0 still at %d "+
			"(was %d before stop). This stale watermark will mask released "+
			"commits during replay.",
			g.pathChangeBeatByRow[0], stalePathChangeBeat)
	}

	// Replay
	var hlEvents int
	g.highlightHook = func(int, int) { hlEvents++ }
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update during replay: %v", err)
	}
	advancePlaybackByAbs(g, g.grid.MaxDiv()*16)
	g.refreshDrumRow()

	secondActive := countActiveSteps(g, 0)
	if secondActive == 0 {
		t.Fatalf("after edit→stop→replay: row 0 slate is all-grey "+
			"(Steps all false). first had %d active. "+
			"stale pathChangeBeat=%d. steps=%v",
			firstActive, stalePathChangeBeat, g.drum.Rows[0].Steps)
	}
	if hlEvents == 0 {
		t.Fatalf("after edit→stop→replay: no highlight callbacks fired "+
			"(playhead not following). stale pathChangeBeat=%d", stalePathChangeBeat)
	}
}

// TestPauseStopReplayKeepsSlateAndHighlights covers a sibling regression: the
// user pauses, then stops, then replays. The pause→stop transition is the path
// covered by stop_from_paused_clears_visuals_test.go; this adds the replay leg
// to ensure the second playback recovers the slate and highlights the same way
// as the first. The internal state on entry to second-Play differs from a
// pure play→stop→play cycle (state.justPaused was set, etc.), so this guards
// the alternate path.
func TestPauseStopReplayKeepsSlateAndHighlights(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)
	g.drum.SetFollow(true)
	g.drum.SetLength(64)

	const loopLen = 4
	nodes := make([]*uiNode, loopLen)
	for i := 0; i < loopLen; i++ {
		nodes[i] = g.tryAddNode(i, 0, model.NodeTypeRegular)
		if i > 0 {
			g.addEdge(nodes[i-1], nodes[i])
		}
	}
	g.addEdge(nodes[loopLen-1], nodes[0])
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.drum.Rows[0].Origin = nodes[0].ID
	g.drum.Rows[0].Node = nodes[0]

	g.drum.SetBPM(120)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Play
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update during play: %v", err)
	}
	if !g.Playing() {
		t.Fatalf("expected playing after first play")
	}
	advancePlaybackByAbs(g, g.grid.MaxDiv()*8)
	g.refreshDrumRow()
	firstActive := countActiveSteps(g, 0)
	if firstActive == 0 {
		t.Fatalf("first playback: row 0 has no active cells")
	}

	// Pause (toggle play while playing)
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update during pause: %v", err)
	}
	if !g.Paused() {
		t.Fatalf("expected paused after toggling play")
	}

	// Stop from paused
	pressStop(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update on stop from paused: %v", err)
	}
	if g.Playing() || g.Paused() {
		t.Fatalf("expected fully stopped after stop")
	}

	// Replay
	var hlEvents int
	g.highlightHook = func(int, int) { hlEvents++ }
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update during replay: %v", err)
	}
	if !g.Playing() {
		t.Fatalf("expected playing after replay")
	}
	advancePlaybackByAbs(g, g.grid.MaxDiv()*8)
	g.refreshDrumRow()

	secondActive := countActiveSteps(g, 0)
	if secondActive == 0 {
		t.Fatalf("after pause→stop→replay: row 0 slate is all-grey "+
			"(Steps all false). first had %d active. steps=%v",
			firstActive, g.drum.Rows[0].Steps)
	}
	if hlEvents == 0 {
		t.Fatalf("after pause→stop→replay: no highlight callbacks fired")
	}
	if g.elapsedBeats == 0 {
		t.Fatalf("after pause→stop→replay: elapsedBeats stuck at 0 (playhead frozen)")
	}
}
