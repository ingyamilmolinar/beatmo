//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Through each control's production OnClick, which catches a handler that mutates the graph without recording.
type sidebarUndoCase struct {
	name string
	// Runs before the undo baseline is taken.
	prep func(t *testing.T, g *Game, node *uiNode)
	// fire is the sidebar button id whose production OnClick is invoked.
	fire sbID
}

// Pure-UI controls (close, the dropdown toggles, move) change no exported state and are absent.
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
			// Import reads groove_pct only alongside a groove kind, so one is set.
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

// OnClick only enqueues; the mutation and its undo tap happen in the drain, inside the per-frame bracket as in
// Update.
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

// For every document-mutating control: it changes the document, records EXACTLY ONE step, and Undo / Redo restore
// the before / after documents.
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

			// Normalised through import, as the all-actions sweep does; restoring suppresses recording.
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
