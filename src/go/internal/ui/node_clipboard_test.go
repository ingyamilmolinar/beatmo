package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newClipboardTestGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	return g
}

func TestCopySelectedNodePopulatesClipboardAndEmits(t *testing.T) {
	g := newClipboardTestGame(t)
	n := g.tryAddNode(20, 20, 0) // regular; auto-selects → g.sel = n
	// Give the node non-default params to prove they are captured.
	node, _ := g.graph.GetNodeByID(n.ID)
	node.Params.Volume = 0.42
	node.Params.Pitch = 7
	node.Params.LogicKind = "probability"
	node.Params.LogicP = 0.5
	node.Params.EffectOverrides = []model.EffectOverride{{Type: "distortion", Param: "drive", Value: 0.8}}
	g.graph.Nodes[n.ID] = node

	before := len(g.nodes)
	e := awaitEmit(t, hooks.EventNodeCopied, func() { g.copySelectedNode() })

	if !g.clipboard.has {
		t.Fatal("clipboard not populated after copy")
	}
	if g.clipboard.params.Volume != 0.42 || g.clipboard.params.Pitch != 7 {
		t.Fatalf("clipboard params not captured: %+v", g.clipboard.params)
	}
	if len(g.nodes) != before {
		t.Fatalf("copy changed the document: nodes %d -> %d", before, len(g.nodes))
	}
	if e.Kind != hooks.EventNodeCopied {
		t.Fatalf("wrong event kind %v", e.Kind)
	}

	// Independence: mutating the source node must not change the clipboard.
	node2, _ := g.graph.GetNodeByID(n.ID)
	node2.Params.Volume = 0.99
	node2.Params.EffectOverrides[0].Value = 0.1
	g.graph.Nodes[n.ID] = node2
	if g.clipboard.params.Volume != 0.42 || g.clipboard.params.EffectOverrides[0].Value != 0.8 {
		t.Fatal("clipboard aliased the source node params (deep copy failed)")
	}
}

// withStubbedKeys swaps the isKeyPressed/isKeyJustPressed vars for the duration
// of fn: keys in pressed report IsKeyPressed==true; keys in just report
// IsKeyJustPressed==true (and IsKeyPressed==true too, matching real hardware).
func withStubbedKeys(pressed, just []ebiten.Key, fn func()) {
	oldP, oldJ := isKeyPressed, isKeyJustPressed
	defer func() { isKeyPressed, isKeyJustPressed = oldP, oldJ }()
	set := func(keys []ebiten.Key) map[ebiten.Key]bool {
		m := map[ebiten.Key]bool{}
		for _, k := range keys {
			m[k] = true
		}
		return m
	}
	pm, jm := set(pressed), set(just)
	isKeyPressed = func(k ebiten.Key) bool { return pm[k] || jm[k] }
	isKeyJustPressed = func(k ebiten.Key) bool { return jm[k] }
	fn()
}

func TestCtrlCCopiesViaKeyboard(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(21, 21, 0) // auto-selects
	withStubbedKeys([]ebiten.Key{ebiten.KeyControlLeft}, []ebiten.Key{ebiten.KeyC}, func() {
		g.handleUndoRedoKeys()
	})
	if !g.clipboard.has {
		t.Fatal("Ctrl-C did not populate the clipboard")
	}
}

func TestCmdCCopiesViaKeyboard(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(22, 22, 0)
	withStubbedKeys([]ebiten.Key{ebiten.KeyMetaLeft}, []ebiten.Key{ebiten.KeyC}, func() {
		g.handleUndoRedoKeys()
	})
	if !g.clipboard.has {
		t.Fatal("Cmd-C did not populate the clipboard")
	}
}

func TestCtrlRRedoesViaKeyboard(t *testing.T) {
	g := newClipboardTestGame(t)
	n := g.tryAddNode(23, 23, 0)
	base := len(g.nodes)
	g.deleteNode(n) // undoable
	if len(g.nodes) != base-1 {
		t.Fatalf("delete failed: %d", len(g.nodes))
	}
	g.performUndo() // node restored
	if len(g.nodes) != base {
		t.Fatalf("undo failed: %d", len(g.nodes))
	}
	withStubbedKeys([]ebiten.Key{ebiten.KeyControlLeft}, []ebiten.Key{ebiten.KeyR}, func() {
		g.handleUndoRedoKeys()
	})
	if len(g.nodes) != base-1 {
		t.Fatalf("Ctrl-R did not redo the delete: nodes=%d want=%d", len(g.nodes), base-1)
	}
}

func TestCutRemovesNodeAndFillsClipboardAndUndoRestores(t *testing.T) {
	g := newClipboardTestGame(t)
	n := g.tryAddNode(24, 24, 0)
	base := len(g.nodes)
	id := n.ID

	e := awaitEmit(t, hooks.EventNodeCut, func() { g.cutSelectedNode() })
	if e.Kind != hooks.EventNodeCut {
		t.Fatalf("cut emitted wrong event kind %v", e.Kind)
	}

	if g.nodeByID(id) != nil {
		t.Fatal("cut did not remove the node from the graph")
	}
	if len(g.nodes) != base-1 {
		t.Fatalf("node count after cut = %d; want %d", len(g.nodes), base-1)
	}
	if !g.clipboard.has {
		t.Fatal("cut did not populate the clipboard")
	}
	g.performUndo()
	if len(g.nodes) != base {
		t.Fatalf("undo after cut did not restore the node: %d", len(g.nodes))
	}
}

// pasteAtCursor reads cursorPosition() then guideTargetAt(). Stub the cursor to
// point at a grid cell by driving the guide resolver directly via the camera is
// complex; instead we place the cursor and assert on the resolved cell by
// choosing an empty target and reading the pasted node.
func withStubbedCursor(x, y int, fn func()) {
	old := cursorPosition
	defer func() { cursorPosition = old }()
	cursorPosition = func() (int, int) { return x, y }
	fn()
}

func TestPasteIntoEmptyCellCreatesIndependentCopyNoEdges(t *testing.T) {
	g := newClipboardTestGame(t)
	src := g.tryAddNode(30, 30, 0)
	node, _ := g.graph.GetNodeByID(src.ID)
	node.Params.Volume = 0.33
	node.Params.Pitch = 5
	g.graph.Nodes[src.ID] = node
	g.copySelectedNode()

	// Paste directly at a known-empty cell (bypass cursor->guide resolution by
	// calling pasteNodeFrom, which pasteAtCursor delegates to).
	before := len(g.nodes)
	pasted := g.pasteNodeFrom(40, 40, g.clipboard)
	if pasted == nil {
		t.Fatal("pasteNodeFrom returned nil")
	}
	if len(g.nodes) != before+1 {
		t.Fatalf("paste did not add a node: %d -> %d", before, len(g.nodes))
	}
	pn, _ := g.graph.GetNodeByID(pasted.ID)
	if pn.Params.Volume != 0.33 || pn.Params.Pitch != 5 {
		t.Fatalf("pasted params not copied: %+v", pn.Params)
	}
	if pasted.ID == src.ID {
		t.Fatal("pasted node reused the source NodeID")
	}
	if g.sel != pasted {
		t.Fatal("paste did not select the new node")
	}
	// No edges on the pasted node.
	for e := range g.graph.Edges {
		if e[0] == pasted.ID || e[1] == pasted.ID {
			t.Fatalf("pasted node has an edge %v; want none", e)
		}
	}
}

func TestPasteWithEmptyClipboardIsNoOp(t *testing.T) {
	g := newClipboardTestGame(t)
	before := len(g.nodes)
	g.pasteAtCursor() // clipboard empty
	if len(g.nodes) != before {
		t.Fatalf("empty-clipboard paste changed node count: %d -> %d", before, len(g.nodes))
	}
}

func TestPasteAtCursorUndoRestores(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(31, 31, 0)
	g.copySelectedNode()
	base := len(g.nodes)
	g.pasteNodeFrom(41, 41, g.clipboard)
	if len(g.nodes) != base+1 {
		t.Fatalf("paste failed: %d", len(g.nodes))
	}
	g.performUndo()
	if len(g.nodes) != base {
		t.Fatalf("undo after paste did not remove the node: %d", len(g.nodes))
	}
}

// TestPasteAtCursorPastesAtResolvedEmptyCell drives the REAL pasteAtCursor
// glue: it stubs the cursor, lets guideTargetAt resolve the grid cell under
// that cursor, and asserts a paste lands at that resolved cell. This covers the
// cursorPosition()->guideTargetAt()->pasteNodeFrom path (the occupied-cell
// branch is covered in Task 6).
func TestPasteAtCursorPastesAtResolvedEmptyCell(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(32, 32, 0) // source node, auto-selected
	g.copySelectedNode()
	withStubbedCursor(400, 300, func() {
		gt := g.guideTargetAt(400, 300)
		if existing := g.nodeAt(gt.I, gt.J); existing != nil {
			t.Fatalf("test precondition failed: cursor-resolved cell (%d,%d) is occupied", gt.I, gt.J)
		}
		before := len(g.nodes)
		g.pasteAtCursor()
		if len(g.nodes) != before+1 {
			t.Fatalf("pasteAtCursor did not paste at resolved empty cell: nodes %d -> %d", before, len(g.nodes))
		}
		if g.nodeAt(gt.I, gt.J) == nil {
			t.Fatalf("no node at cursor-resolved cell (%d,%d) after pasteAtCursor", gt.I, gt.J)
		}
	})
}

func TestPasteOverOccupiedOpensConfirmNoImmediateChange(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(50, 50, 0) // source, selected
	g.copySelectedNode()
	occupant := g.tryAddNode(60, 60, 0) // target cell occupied
	base := len(g.nodes)

	g.beginPasteConfirm(60, 60)

	if !g.pasteConfirmActive() {
		t.Fatal("confirm not active after paste over occupied cell")
	}
	if len(g.nodes) != base {
		t.Fatalf("occupied paste mutated the graph before confirm: %d -> %d", base, len(g.nodes))
	}
	if g.nodeByID(occupant.ID) == nil {
		t.Fatal("occupant deleted before confirmation")
	}
}

func TestConfirmPasteOverwriteReplacesInOneUndoStep(t *testing.T) {
	g := newClipboardTestGame(t)
	src := g.tryAddNode(51, 51, 0)
	node, _ := g.graph.GetNodeByID(src.ID)
	node.Params.Pitch = 9
	g.graph.Nodes[src.ID] = node
	g.copySelectedNode()
	occupant := g.tryAddNode(61, 61, 0)
	occID := occupant.ID
	base := len(g.nodes)

	g.beginPasteConfirm(61, 61)
	g.ConfirmPasteOverwrite()

	if g.pasteConfirmActive() {
		t.Fatal("confirm still active after overwrite")
	}
	if g.nodeByID(occID) != nil {
		t.Fatal("occupant not replaced")
	}
	if len(g.nodes) != base {
		t.Fatalf("replace changed node count: %d -> %d", base, len(g.nodes))
	}
	pn := g.nodeAt(61, 61)
	if pn == nil {
		t.Fatal("no node at target after overwrite")
	}
	m, _ := g.graph.GetNodeByID(pn.ID)
	if m.Params.Pitch != 9 {
		t.Fatalf("overwritten node has wrong params: %+v", m.Params)
	}
	// One undo step: a single undo reverses the whole replace (occupant back).
	g.performUndo()
	if g.nodeByID(occID) == nil {
		t.Fatal("single undo did not restore the occupant (replace was not one step)")
	}
}

func TestCancelPasteOverwriteLeavesGraphUnchanged(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(52, 52, 0)
	g.copySelectedNode()
	occupant := g.tryAddNode(62, 62, 0)
	base := len(g.nodes)

	g.beginPasteConfirm(62, 62)
	g.CancelPasteOverwrite()

	if g.pasteConfirmActive() {
		t.Fatal("confirm still active after cancel")
	}
	if g.nodeByID(occupant.ID) == nil || len(g.nodes) != base {
		t.Fatal("cancel mutated the graph")
	}
}

func TestEscCancelsPasteConfirm(t *testing.T) {
	g := newClipboardTestGame(t)
	g.tryAddNode(53, 53, 0)
	g.copySelectedNode()
	g.tryAddNode(63, 63, 0)
	g.beginPasteConfirm(63, 63)

	withStubbedKeys(nil, []ebiten.Key{ebiten.KeyEscape}, func() {
		g.handleEscape()
	})
	if g.pasteConfirmActive() {
		t.Fatal("Esc did not cancel the paste confirmation")
	}
}

// TestEscClosesOnlyPasteConfirmNotUnderlyingPortal is a regression test for a
// review-found bug: openPasteConfirmPortal's escapeFn used to call
// CancelPasteOverwrite() (which does an explicit portal Close(id) on the
// paste-confirm entry) and then return false, letting handleEscape's
// unconditional fallback CloseTop() pop whatever portal was now topmost — a
// DIFFERENT portal sitting beneath the paste-confirm (reachable because
// Ctrl+V is ungated and can stack the confirm over an already-open popup).
// The fix makes escapeFn cleanup-only (clear g.pastePending, return false) so
// CloseTop removes the paste-confirm entry itself and nothing else.
func TestEscClosesOnlyPasteConfirmNotUnderlyingPortal(t *testing.T) {
	g := newClipboardTestGame(t)
	p := g.drum.portal()
	if p == nil {
		t.Skip("no portal in this test game")
	}
	dummyClosed := false
	// A minimal underlying modal portal opened FIRST (sits beneath paste-confirm).
	p.Open(PortalEntry{
		ID:    "dummy-under",
		Modal: true,
		Overlay: &dvOverlayPortal{
			id:       "dummy-under",
			isOpenFn: func() bool { return true },
			rectFn:   func() image.Rectangle { return image.Rect(0, 0, 10, 10) },
			drawFn:   func(*ebiten.Image) {},
			inputFn:  func(x, y int, pressed bool) InputResult { return InputConsumed },
		},
		OnClose: func() { dummyClosed = true },
	})
	// Arm + open paste-confirm ON TOP (occupied-cell path).
	g.tryAddNode(50, 50, 0)
	g.copySelectedNode()
	g.tryAddNode(60, 60, 0)
	g.beginPasteConfirm(60, 60)

	// Esc.
	withStubbedKeys(nil, []ebiten.Key{ebiten.KeyEscape}, func() { g.handleEscape() })

	if g.pasteConfirmActive() {
		t.Fatal("Esc did not cancel the paste-confirm")
	}
	if dummyClosed {
		t.Fatal("Esc wrongly closed the underlying portal (double-close bug)")
	}
}

// TestPasteOnEdgeStitchesIntoConnection verifies that pasting a node onto a
// grid cell lying on an existing (orthogonal) edge auto-wires the pasted node
// into that connection — the same behavior as creating a node on an edge via
// tryAddNode (which calls stitchEdgesAt). Edge A-B is split into A-n and n-B.
func TestPasteOnEdgeStitchesIntoConnection(t *testing.T) {
	g := newClipboardTestGame(t)
	hasEdge := func(a, b *uiNode) bool {
		_, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]
		return ok
	}
	// Horizontal edge A(3,3) -> B(6,3).
	a := g.tryAddNode(3, 3, model.NodeTypeRegular)
	b := g.tryAddNode(6, 3, model.NodeTypeRegular)
	g.addEdge(a, b)
	if !hasEdge(a, b) {
		t.Fatal("precondition: A-B edge not created")
	}
	// A source node away from the edge, copied to the clipboard.
	g.tryAddNode(3, 9, model.NodeTypeRegular) // auto-selected
	g.copySelectedNode()

	// (4,3) lies strictly on the A-B horizontal edge.
	n := g.pasteNodeFrom(4, 3, g.clipboard)
	if n == nil {
		t.Fatal("paste returned nil")
	}
	if hasEdge(a, b) {
		t.Fatal("A-B edge still present: paste did not stitch into the active edge")
	}
	if !hasEdge(a, n) || !hasEdge(n, b) {
		t.Fatalf("pasted node not stitched into edge: A-n=%v n-B=%v (want both true)", hasEdge(a, n), hasEdge(n, b))
	}

	// Atomic undo: a single Ctrl-Z restores the original A-B edge and removes n.
	g.performUndo()
	if !hasEdge(a, b) {
		t.Fatal("undo did not restore the original A-B edge")
	}
	if g.nodeByID(n.ID) != nil {
		t.Fatal("undo did not remove the pasted node (stitched paste not one atomic step)")
	}
}

// TestPasteConfirm_OutsideClickCancels drives the shared ConfirmDialog's
// click-outside contract through the REAL input path: a press well outside
// the centered dialog panel yields zero hit-area matches, so the tree's
// CloseTop → OnClose fires (ConfirmDialog.portalEntry wires OnClose to
// d.cancel → g.CancelPasteOverwrite), leaving the graph untouched.
func TestPasteConfirm_OutsideClickCancels(t *testing.T) {
	g := newTestGameForUndo(t)
	n := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.updateBeatInfos()
	if n == nil {
		t.Fatal("setup node nil")
	}
	g.snapshotNodeToClipboard(n)
	g.beginPasteConfirm(3, 1)
	if !g.pasteConfirmActive() {
		t.Fatal("paste confirm should be active")
	}
	// A press well outside the dialog panel (screen top-left corner) must
	// cancel via the portal's outside-click path.
	clickAt(g, 2, 2)
	if g.pasteConfirmActive() {
		t.Fatal("outside click must cancel the paste confirm")
	}
	if g.nodeAt(3, 1) != n {
		t.Fatal("cancel must not mutate the graph")
	}
}
