//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestConnectModeBasicEdge verifies that connect mode creates a horizontal
// edge between two perpendicular nodes and exits.
func TestConnectModeBasicEdge(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(8, 0, model.NodeTypeRegular)
	edgesBefore := len(g.edges)

	g.enterConnectMode(a)
	if !g.connectMode {
		t.Fatal("connectMode should be true")
	}

	// Simulate tap on node B's screen position.
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(b)
	bx := int((sx1 + sx2) * 0.5)
	by := int((sy1 + sy2) * 0.5)
	g.handleConnectModeTap(bx, by)

	if g.connectMode {
		t.Fatal("connectMode should be cancelled after successful edge")
	}
	if len(g.edges) != edgesBefore+1 {
		t.Fatalf("expected %d edges, got %d", edgesBefore+1, len(g.edges))
	}
}

// TestConnectModeVerticalEdge verifies that connect mode works for vertical
// (same I) node pairs.
func TestConnectModeVerticalEdge(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(0, 8, model.NodeTypeRegular)
	edgesBefore := len(g.edges)

	g.enterConnectMode(a)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(b)
	bx := int((sx1 + sx2) * 0.5)
	by := int((sy1 + sy2) * 0.5)
	g.handleConnectModeTap(bx, by)

	if g.connectMode {
		t.Fatal("connectMode should be cancelled after successful edge")
	}
	if len(g.edges) != edgesBefore+1 {
		t.Fatalf("expected %d edges, got %d", edgesBefore+1, len(g.edges))
	}
}

// TestConnectModeNonPerpendicularError verifies that tapping a diagonal node
// shows an error notification and keeps connect mode active.
func TestConnectModeNonPerpendicularError(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 4, model.NodeTypeRegular)
	edgesBefore := len(g.edges)

	g.enterConnectMode(a)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(b)
	bx := int((sx1 + sx2) * 0.5)
	by := int((sy1 + sy2) * 0.5)
	g.handleConnectModeTap(bx, by)

	if !g.connectMode {
		t.Fatal("connectMode should still be active after non-perpendicular tap")
	}
	if len(g.edges) != edgesBefore {
		t.Fatalf("no edge should have been created; got %d edges", len(g.edges))
	}
	// Verify error notification was shown.
	if g.drum.notifStore.Len() == 0 {
		t.Fatal("expected an error notification")
	}
	last := g.drum.notifStore.Latest()
	if !last.isErr {
		t.Fatal("notification should be an error")
	}
}

// TestConnectModeEmptyTapCancels verifies that tapping empty grid space
// cancels connect mode.
func TestConnectModeEmptyTapCancels(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.enterConnectMode(a)

	// Tap on empty space (far from any node).
	g.handleConnectModeTap(600, 300)

	if g.connectMode {
		t.Fatal("connectMode should be cancelled after empty tap")
	}
}

// TestConnectModeEscCancels verifies that pressing ESC cancels connect mode.
func TestConnectModeEscCancels(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.enterConnectMode(a)

	// Esc is now owned by handleEscape (Task 6 consolidation); it is dispatched
	// every frame from handleGlobalShortcuts in production. Drive that single
	// authority directly instead of handleEditor (which no longer touches Esc).
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	defer restore()

	g.handleEscape()

	if g.connectMode {
		t.Fatal("connectMode should be cancelled after ESC")
	}
}

// TestConnectModeEdgeDuplicate verifies that connecting nodes that already
// share an edge does not create a duplicate.
func TestConnectModeEdgeDuplicate(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(8, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	edgesBefore := len(g.edges)

	g.enterConnectMode(a)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(b)
	bx := int((sx1 + sx2) * 0.5)
	by := int((sy1 + sy2) * 0.5)
	g.handleConnectModeTap(bx, by)

	if g.connectMode {
		t.Fatal("connectMode should be cancelled")
	}
	if len(g.edges) != edgesBefore {
		t.Fatalf("expected %d edges (no duplicate), got %d", edgesBefore, len(g.edges))
	}
}

// TestConnectModePlaybackSafe verifies that creating an edge during playback
// works and playback continues.
func TestConnectModePlaybackSafe(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(8, 0, model.NodeTypeRegular)
	c := g.tryAddNode(16, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.updateBeatInfos()
	g.refreshDrumRow()

	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.SetPlaying(true)
	advanceFrames(g, 10)

	g.enterConnectMode(b)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(c)
	cx := int((sx1 + sx2) * 0.5)
	cy := int((sy1 + sy2) * 0.5)
	g.handleConnectModeTap(cx, cy)

	if g.connectMode {
		t.Fatal("connectMode should be cancelled after edge")
	}
	advanceFrames(g, 10)
	if !g.Playing() {
		t.Fatal("playback should continue after connect mode edge creation")
	}
	stopPlaybackForTest(g)
}

// TestConnectModeClearsOnImport verifies that import cancels connect mode.
func TestConnectModeClearsOnImport(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.enterConnectMode(a)

	if !g.connectMode {
		t.Fatal("connectMode should be active")
	}

	// Import minimal valid JSON.
	data := []byte(`{"version":1,"subdiv":8,"bpm":120,"instruments":[{"name":"Kick","id":"kick","kind":"builtin","volume":1,"origin":0,"color":"#C87850FF"}],"nodes":[{"id":0,"i":0,"j":0,"type":"regular","inputs":[],"outputs":[]}]}`)
	err := g.Import(data)
	if err != nil {
		t.Fatalf("import error: %v", err)
	}

	if g.connectMode {
		t.Fatal("connectMode should be cleared after import")
	}
}

// TestConnectModeSelfConnectionIgnored verifies that tapping the source node
// itself is a no-op (stays in connect mode).
func TestConnectModeSelfConnectionIgnored(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	edgesBefore := len(g.edges)

	g.enterConnectMode(a)
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(a)
	ax := int((sx1 + sx2) * 0.5)
	ay := int((sy1 + sy2) * 0.5)
	g.handleConnectModeTap(ax, ay)

	if !g.connectMode {
		t.Fatal("connectMode should still be active after self-tap")
	}
	if len(g.edges) != edgesBefore {
		t.Fatalf("no edge should be created for self-connection")
	}
}
