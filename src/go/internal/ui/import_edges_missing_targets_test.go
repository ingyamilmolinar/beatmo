package ui

import (
	"encoding/json"
	"testing"
)

// Ensure missing outputs in JSON are ignored gracefully.
func TestImportIgnoresMissingEdgeTargets(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	type node struct {
		ID      int    `json:"id"`
		I       int    `json:"i"`
		J       int    `json:"j"`
		Type    string `json:"type"`
		Outputs []int  `json:"outputs,omitempty"`
	}
	type file struct {
		Version     int    `json:"version"`
		Subdiv      int    `json:"subdiv"`
		BPM         int    `json:"bpm"`
		Nodes       []node `json:"nodes"`
		Instruments []struct {
			Name, ID, Kind, Color string
			Volume                float64
			Origin                int
		} `json:"instruments"`
	}
	f := file{Version: 1, Subdiv: 32, BPM: 120,
		Instruments: []struct {
			Name, ID, Kind, Color string
			Volume                float64
			Origin                int
		}{{Name: "R", ID: "snare", Kind: "builtin", Color: "#FFFFFFFF", Volume: 1, Origin: 1}},
		Nodes: []node{{ID: 1, I: 0, J: 0, Type: "regular", Outputs: []int{999}}},
	}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if len(g.graph.Edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(g.graph.Edges))
	}
}
