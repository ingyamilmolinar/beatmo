package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Ensure removed skip_* rules are mapped to canonical trigger_* rules on import
// and that selecting None clears logic kind.
func TestImportMapsRemovedSkipRulesAndNoneClearsLegacy(t *testing.T) {
	assertDefaultParityState(t)
	// Build a file with two nodes: b uses removed skip_if_prev_triggered,
	// c uses removed skip_if_prev_skipped. Also include a node with legacy SkipEvery.
	file := exportFile{
		Version: 1, Subdiv: 32, BPM: 120,
		Instruments: []exportInstrument{{Name: "Row", ID: "snare", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF"}},
		Nodes: []exportNode{
			{ID: 1, I: 0, J: 0, Type: "regular"},
			{ID: 2, I: 1, J: 0, Type: "regular", LogicKind: "skip_if_prev_triggered"},
			{ID: 3, I: 2, J: 0, Type: "regular", LogicKind: "skip_if_prev_skipped"},
			{ID: 4, I: 3, J: 0, Type: "regular", SkipEvery: 2},
		},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Verify mapping
	var node2, node3, node4 model.Node
	for _, n := range g.graph.Nodes {
		if n.I == 1 && n.J == 0 {
			node2 = n
		}
		if n.I == 2 && n.J == 0 {
			node3 = n
		}
		if n.I == 3 && n.J == 0 {
			node4 = n
		}
	}
	if node2.Params.LogicKind != "trigger_if_prev_skipped" {
		t.Fatalf("expected node2 mapped to trigger_if_prev_skipped, got %q", node2.Params.LogicKind)
	}
	if node3.Params.LogicKind != "trigger_if_prev_triggered" {
		t.Fatalf("expected node3 mapped to trigger_if_prev_triggered, got %q", node3.Params.LogicKind)
	}
	if node4.Params.LogicKind != "skip_every_n" || node4.Params.LogicN != 2 {
		t.Fatalf("expected legacy SkipEvery upgraded to skip_every_n; got kind=%q n=%d", node4.Params.LogicKind, node4.Params.LogicN)
	}
	// Now select node4 and choose None: logic should be cleared
	// find ui node for node4
	var u4 *uiNode
	for _, u := range g.nodes {
		if u.I == node4.I && u.J == node4.J {
			u4 = u
			break
		}
	}
	if u4 == nil {
		t.Fatalf("ui node not found for node4")
	}
	g.sel = u4
	u4.Selected = true
	g.sidebar.Open(u4)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()
	// Open logic dropdown and pick None
	if b := g.sidebar.btns["logic"]; b == nil || !b.Handle((g.sidebar.rects["logic"].Min.X+g.sidebar.rects["logic"].Max.X)/2, (g.sidebar.rects["logic"].Min.Y+g.sidebar.rects["logic"].Max.Y)/2, true) {
		t.Fatalf("logic button click failed")
	}
	_ = g.Update()
	g.sidebar.layout()
	if b := g.sidebar.btns["logic:"]; b == nil || !b.Handle((g.sidebar.rects["logic:"].Min.X+g.sidebar.rects["logic:"].Max.X)/2, (g.sidebar.rects["logic:"].Min.Y+g.sidebar.rects["logic:"].Max.Y)/2, true) {
		t.Fatalf("none item click failed")
	}
	_ = g.Update()
	// Ensure logic cleared
	if n, ok := g.graph.GetNodeByID(u4.ID); ok {
		if n.Params.LogicKind != "" {
			t.Fatalf("LogicKind not cleared on None: %q", n.Params.LogicKind)
		}
	}
}
