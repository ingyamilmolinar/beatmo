//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// sidebarUndoCase drives ONE real node-sidebar ("main node pop-up menu") control
// through its production OnClick closure and asserts the resulting document
// mutation is undoable. Unlike the direct emit-call cases in
// undo_all_actions_test.go, these exercise the actual button handlers in
// game_node_sidebar.go end-to-end, so they catch handlers that mutate the graph
// without recording an undo step.
type sidebarUndoCase struct {
	name string
	// prep establishes prerequisite node state and the sidebar UI state
	// (open sections / open dropdowns) required for the target control to be
	// wired. It runs before the undo baseline is taken.
	prep func(t *testing.T, g *Game, node *uiNode)
	// fire is the sidebar button id whose production OnClick is invoked.
	fire sbID
}

// sidebarUndoCases enumerates EVERY document-mutating control reachable from the
// node sidebar. Pure-UI controls (close, the logic/groove dropdown toggles, the
// move button) are intentionally absent — they change no exported state.
func sidebarUndoCases() []sidebarUndoCase {
	setLogic := func(kind model.LogicKind, n int, p float64) func(*testing.T, *Game, *uiNode) {
		return func(t *testing.T, g *Game, node *uiNode) {
			mn, ok := g.graph.GetNodeByID(node.ID)
			if !ok {
				t.Fatal("node missing")
			}
			ps := mn.Params
			ps.LogicKind = kind
			ps.LogicN = n
			ps.LogicP = p
			g.graph.SetNodeParams(node.ID, ps)
			g.notifyPredictorNode(node.ID)
			g.sidebar.logicDropdownOpen = false
		}
	}
	setGroove := func(kind model.GrooveKind, pct float64) func(*testing.T, *Game, *uiNode) {
		return func(t *testing.T, g *Game, node *uiNode) {
			mn, ok := g.graph.GetNodeByID(node.ID)
			if !ok {
				t.Fatal("node missing")
			}
			ps := mn.Params
			ps.GrooveKind = kind
			ps.GroovePct = pct
			g.graph.SetNodeParams(node.ID, ps)
			g.notifyPredictorNode(node.ID)
			g.sidebar.grooveDropdownOpen = false
		}
	}
	return []sidebarUndoCase{
		{name: "volume-down", fire: sbMinus(fieldVol)},
		{name: "volume-up", fire: sbPlus(fieldVol)},
		{name: "pitch-down", fire: sbMinus(fieldPit)},
		{name: "pitch-up", fire: sbPlus(fieldPit)},
		{name: "duration-down", fire: sbMinus(fieldDur)},
		{name: "duration-up", fire: sbPlus(fieldDur)},
		{
			name: "logic-kind-probability",
			prep: func(t *testing.T, g *Game, node *uiNode) { g.sidebar.logicDropdownOpen = true },
			fire: sbLogicOption(model.LogicProbability),
		},
		{
			name: "logic-kind-every-n",
			prep: func(t *testing.T, g *Game, node *uiNode) { g.sidebar.logicDropdownOpen = true },
			fire: sbLogicOption(model.LogicEveryN),
		},
		{
			name: "logic-n-down",
			prep: setLogic(model.LogicEveryN, 3, 0),
			fire: sbMinus(fieldLogicN),
		},
		{
			name: "logic-n-up",
			prep: setLogic(model.LogicEveryN, 3, 0),
			fire: sbPlus(fieldLogicN),
		},
		{
			name: "logic-p-down",
			prep: setLogic(model.LogicProbability, 0, 0.5),
			fire: sbMinus(fieldLogicP),
		},
		{
			name: "logic-p-up",
			prep: setLogic(model.LogicProbability, 0, 0.5),
			fire: sbPlus(fieldLogicP),
		},
		{
			name: "groove-kind-delay",
			prep: func(t *testing.T, g *Game, node *uiNode) { g.sidebar.grooveDropdownOpen = true },
			fire: sbGrooveOption(model.GrooveDelay),
		},
		{
			// Groove percentage only round-trips through the snapshot when a groove
			// kind is set (export writes groove_pct unconditionally but import reads
			// it only alongside a non-empty groove_kind — a separate import gap). A
			// groove percentage is only meaningful with a kind anyway, so set one.
			name: "groove-pct-up",
			prep: setGroove(model.GrooveDelay, 0.3),
			fire: sbPlus(fieldGroovePct),
		},
		{
			name: "groove-pct-down",
			prep: setGroove(model.GrooveDelay, 0.5),
			fire: sbMinus(fieldGroovePct),
		},
		{
			name: "audible-toggle",
			fire: sbAudible,
		},
	}
}

// fireSidebarControl lays out the sidebar (wiring its button closures), invokes
// the named control's production OnClick, then drains the UI queue exactly the
// way Game.Update does — inside the per-frame undo bracket (game_update.go). This
// is the real execution path: OnClick only enqueues; the mutation (and any undo
// tap) happens during the drain.
func (g *Game) fireSidebarControl(t *testing.T, id sbID) {
	t.Helper()
	g.sidebar.layout()
	btn, ok := g.sidebar.btns[id]
	if !ok || btn == nil {
		t.Fatalf("sidebar control %q is not wired (rect missing or section/dropdown closed)", id)
	}
	if btn.OnClick == nil {
		t.Fatalf("sidebar control %q has no OnClick", id)
	}
	btn.OnClick()

	// Drain the UI queue inside a per-frame undo bracket, mirroring Game.Update.
	beginUndoGroup("")
	q := g.uiQueue
	g.uiQueue = nil
	for _, fn := range q {
		if fn != nil {
			fn()
		}
	}
	endUndoGroup()
}

// TestNodeSidebarControlsAreUndoable proves that every document-mutating control
// in the node pop-up menu records an atomic, reversible undo step. For each
// control it asserts:
//
//  1. firing the real handler changes the exported document,
//  2. it records EXACTLY ONE undo step (atomic), and
//  3. Undo restores the pre-action document byte-for-byte, and Redo re-applies
//     the post-action document.
func TestNodeSidebarControlsAreUndoable(t *testing.T) {
	for _, tc := range sidebarUndoCases() {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGameForUndo(t)
			g.tryAddNode(7, 7, model.NodeTypeRegular)
			node := g.nodeAt(7, 7)
			if node == nil {
				t.Fatal("failed to add node")
			}
			g.sidebar.Open(node)
			g.sidebar.ExpandAllSections()
			if tc.prep != nil {
				tc.prep(t, g, node)
			}
			g.updateBeatInfos()

			// Baseline = the document right before firing the control.
			g.undoManager.OnExternalLoad()
			before := g.undoCapture()
			depth0 := len(g.undoManager.undo)

			g.fireSidebarControl(t, tc.fire)

			after := g.undoCapture()
			if string(after) == string(before) {
				t.Fatalf("%s did not change the exported document", tc.name)
			}
			if steps := len(g.undoManager.undo) - depth0; steps != 1 {
				t.Fatalf("%s recorded %d undo steps, want exactly 1 (atomic)", tc.name, steps)
			}

			g.undoManager.Undo()
			if got := g.undoCapture(); string(got) != string(before) {
				t.Fatalf("%s: Undo not byte-identical to pre-action document", tc.name)
			}

			// Param mutations don't renumber node ids, but normalise through the
			// import path anyway (matching undo_all_actions_test.go) so the Redo
			// comparison is robust. restoring suppresses recording/clearing.
			normalize := func(b []byte) string {
				g.undoManager.restoring = true
				_ = g.Import(b)
				out := g.undoCapture()
				g.undoManager.restoring = false
				return string(out)
			}
			wantRedo := normalize(after)
			g.undoManager.Redo()
			if got := g.undoCapture(); string(got) != wantRedo {
				t.Fatalf("%s: Redo did not restore the post-action document", tc.name)
			}
		})
	}
}
