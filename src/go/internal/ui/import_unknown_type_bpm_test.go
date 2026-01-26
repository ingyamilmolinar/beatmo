package ui

import (
	"encoding/json"
	"testing"
)

// Unknown node types are treated as regular and should not break import.
func TestImportUnknownNodeTypeAndNegativeBPM(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	startBPM := g.drum.BPM()
	type node struct {
		ID, I, J int
		Type     string
	}
	type inst struct {
		Name, ID, Kind, Color string
		Volume                float64
		Origin                int
	}
	f := struct {
		Version     int    `json:"version"`
		Subdiv      int    `json:"subdiv"`
		BPM         int    `json:"bpm"`
		Nodes       []node `json:"nodes"`
		Instruments []inst `json:"instruments"`
	}{Version: 1, Subdiv: 32, BPM: -10,
		Nodes:       []node{{ID: 1, I: 0, J: 0, Type: "mystery"}},
		Instruments: []inst{{Name: "R", ID: "snare", Kind: "builtin", Color: "#FFFFFFFF", Volume: 1, Origin: 1}},
	}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	// BPM unchanged because negative input is ignored
	if g.drum.BPM() != startBPM {
		t.Fatalf("BPM changed unexpectedly: %d", g.drum.BPM())
	}
	// Ensure a single non-invisible node was created and is regular or at least not mute/silent
	got := 0
	for _, n := range g.graph.Nodes {
		if n.Type != 1 { // not Invisible
			got++
			if n.Type == 2 || n.Type == 3 {
				t.Fatalf("unexpected node type: %v", n.Type)
			}
		}
	}
	if got == 0 {
		t.Fatalf("no regular node created from unknown type")
	}
}
