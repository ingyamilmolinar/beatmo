package ui

import (
	"encoding/json"
	"testing"
)

// Ensure edges are created from silent nodes as well as regular nodes.
func TestImportCreatesEdgesFromSilentNodes(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// A square loop where every other node is silent. All edges should be created.
	exp := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Nodes: []exportNode{
			{ID: 0, I: 0, J: 0, Type: "regular", Outputs: []int{1}},
			{ID: 1, I: 1, J: 0, Type: "silent", Outputs: []int{2}},
			{ID: 2, I: 1, J: 1, Type: "regular", Outputs: []int{3}},
			{ID: 3, I: 0, J: 1, Type: "silent", Outputs: []int{0}},
		},
		Instruments: []exportInstrument{
			{Name: "Row0", ID: "snare", Kind: "builtin", Volume: 1, Origin: 0, Color: "#FFFFFFFF"},
		},
	}
	data, _ := json.Marshal(exp)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	// After import, beat path should include silent nodes and form a loop with > 2 steps.
	if len(g.beatInfosByRow) == 0 || len(g.beatInfosByRow[0]) < 4 {
		t.Fatalf("expected beat path >=4, got %d", len(g.beatInfosByRow[0]))
	}
}
