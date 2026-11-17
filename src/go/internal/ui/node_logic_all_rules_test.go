package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// TestImportRejectsRetiredLogicSpellings: a rule has one spelling. A file
// naming a retired one (skip_if_prev_*, prev_fired, every_n_loops, none) is
// rejected before anything resets, instead of being silently remapped.
func TestImportRejectsRetiredLogicSpellings(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	before := len(g.graph.Nodes)
	for _, retired := range []string{"skip_if_prev_triggered", "skip_if_prev_skipped", "prev_fired", "every_n_loops", "none", "Probability"} {
		file := projectFile{
			Envelope: schema.Current(), Subdiv: 32, BPM: 120,
			Instruments: []exportInstrument{{Name: "Row", ID: roster.Snare, Kind: kindBuiltin, Volume: 1, Origin: 1, Color: "#FFFFFFFF"}},
			Nodes:       []exportNode{{ID: 1, I: 0, J: 0, Type: model.NodeTypeRegular, LogicKind: model.LogicProbability}},
		}
		data, _ := json.Marshal(file)
		data = []byte(strings.Replace(string(data), `"logic_kind":"probability"`, `"logic_kind":"`+retired+`"`, 1))
		if err := g.Import(data); err == nil {
			t.Errorf("import accepted logic_kind %q", retired)
		}
		if len(g.graph.Nodes) != before {
			t.Fatalf("a rejected file changed the circuit (%d nodes, had %d)", len(g.graph.Nodes), before)
		}
	}
}

// TestImportKeepsEveryLogicRuleAndNoneClears: every rule survives import
// verbatim, and selecting None in the sidebar clears it.
func TestImportKeepsEveryLogicRuleAndNoneClears(t *testing.T) {
	assertDefaultParityState(t)
	file := projectFile{
		Envelope: schema.Current(), Subdiv: 32, BPM: 120,
		Instruments: []exportInstrument{{Name: "Row", ID: roster.Snare, Kind: kindBuiltin, Volume: 1, Origin: 1, Color: "#FFFFFFFF"}},
		Nodes: []exportNode{
			{ID: 1, I: 0, J: 0, Type: model.NodeTypeRegular},
			{ID: 2, I: 1, J: 0, Type: model.NodeTypeRegular, LogicKind: model.LogicIfPrevSkipped},
			{ID: 3, I: 2, J: 0, Type: model.NodeTypeRegular, LogicKind: model.LogicIfPrevTriggered},
			{ID: 4, I: 3, J: 0, Type: model.NodeTypeRegular, LogicKind: model.LogicSkipEveryN, LogicN: 2},
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
	if node2.Params.LogicKind != model.LogicIfPrevSkipped {
		t.Fatalf("node2 logic = %q, want trigger_if_prev_skipped", node2.Params.LogicKind)
	}
	if node3.Params.LogicKind != model.LogicIfPrevTriggered {
		t.Fatalf("node3 logic = %q, want trigger_if_prev_triggered", node3.Params.LogicKind)
	}
	if node4.Params.LogicKind != model.LogicSkipEveryN || node4.Params.LogicN != 2 {
		t.Fatalf("node4 logic = %q n=%d, want skip_every_n n=2", node4.Params.LogicKind, node4.Params.LogicN)
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
	if b := g.sidebar.btns[sbLogicSelect]; b == nil || b.HandleInputResult((g.sidebar.rects[sbLogicSelect].Min.X+g.sidebar.rects[sbLogicSelect].Max.X)/2, (g.sidebar.rects[sbLogicSelect].Min.Y+g.sidebar.rects[sbLogicSelect].Max.Y)/2, true) == InputIgnored {
		t.Fatalf("logic button click failed")
	}
	_ = g.Update()
	g.sidebar.layout()
	if b := g.sidebar.btns[sbLogicOption(model.LogicNone)]; b == nil || b.HandleInputResult((g.sidebar.rects[sbLogicOption(model.LogicNone)].Min.X+g.sidebar.rects[sbLogicOption(model.LogicNone)].Max.X)/2, (g.sidebar.rects[sbLogicOption(model.LogicNone)].Min.Y+g.sidebar.rects[sbLogicOption(model.LogicNone)].Max.Y)/2, true) == InputIgnored {
		t.Fatalf("none item click failed")
	}
	_ = g.Update()
	// Ensure logic cleared
	if n, ok := g.graph.GetNodeByID(u4.ID); ok {
		if n.Params.LogicKind != model.LogicNone {
			t.Fatalf("LogicKind not cleared on None: %q", n.Params.LogicKind)
		}
	}
}
