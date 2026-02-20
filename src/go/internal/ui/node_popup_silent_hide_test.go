package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Ensure that when a node is toggled to Silent or Mute, the popup hides the
// appropriate informational rows (vol/pitch/dur/logic/groove) leaving only the
// audible toggle where necessary. Switching back to Regular restores the rows
// with previous values.
func TestNodePopupHidesForSilentAndMute_ThenRestores(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Add a regular node and open its menu
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()
	// Sanity: rows visible
	if g.sidebar.rects["vol-"].Empty() || g.sidebar.rects["logic"].Empty() {
		t.Fatalf("expected initial rows visible: vol/logic missing")
	}
	// Remember values to verify state persists across toggles
	before := g.graph.Nodes[n.ID].Params

	// Click AUD to cycle to Silent (invoke handler directly)
	if g.sidebar.btns == nil || g.sidebar.btns["aud"] == nil {
		t.Fatalf("aud button missing")
	}
	g.sidebar.btns["aud"].OnClick()
	_ = g.Update()
	g.sidebar.layout() // recompute rects after type change
	// After Silent: all rows hidden except AUD
	if !g.sidebar.rects["vol-"].Empty() || !g.sidebar.rects["logic"].Empty() || !g.sidebar.rects["dur-"].Empty() || !g.sidebar.rects["grv"].Empty() {
		t.Fatalf("expected rows hidden for Silent")
	}
	// Click AUD again -> Mute (show only non-audible rows such as logic)
	if g.sidebar.btns["aud"] == nil {
		t.Fatalf("aud button missing (silent)")
	}
	g.sidebar.btns["aud"].OnClick()
	_ = g.Update()
	g.sidebar.layout()
	if !g.sidebar.rects["vol-"].Empty() || !g.sidebar.rects["pit-"].Empty() {
		t.Fatalf("volume or pitch still visible for Mute")
	}
	if g.sidebar.rects["logic"].Empty() {
		t.Fatalf("logic should remain visible for Mute")
	}
	if !g.sidebar.rects["dur-"].Empty() || !g.sidebar.rects["dur+"].Empty() {
		t.Fatalf("duration should be hidden for Mute")
	}
	// Click third time -> Regular (recompute rect again)
	if g.sidebar.btns["aud"] == nil {
		t.Fatalf("aud button missing (mute)")
	}
	g.sidebar.btns["aud"].OnClick()
	_ = g.Update()
	g.sidebar.layout()
	// Rows restored
	t.Logf("final types: type=%v vol-=%v logic=%v dur-=%v aud=%v", g.graph.Nodes[n.ID].Type, g.sidebar.rects["vol-"], g.sidebar.rects["logic"], g.sidebar.rects["dur-"], g.sidebar.rects["aud"])
	if g.sidebar.rects["vol-"].Empty() || g.sidebar.rects["logic"].Empty() || g.sidebar.rects["dur-"].Empty() {
		t.Fatalf("rows not restored after returning to Regular")
	}
	// Parameters unchanged
	after := g.graph.Nodes[n.ID].Params
	// Compare scalar fields (Logic function may be non-comparable)
	if before.Volume != after.Volume || before.Pitch != after.Pitch || before.Duration != after.Duration || before.LogicKind != after.LogicKind || before.LogicN != after.LogicN || before.LogicP != after.LogicP || before.GrooveKind != after.GrooveKind || before.GroovePct != after.GroovePct || before.SkipEveryN != after.SkipEveryN {
		t.Fatalf("node params changed across mute/silent toggles: before=%+v after=%+v", before, after)
	}
}
