package ui

import (
	"encoding/json"
	"testing"
)

func TestImportClampsRowAndNodeParams(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// One instrument with excessive volume; one node with invalid params.
	// Local node with groove fields for this test
	type node2 struct {
		ID         int     `json:"id"`
		I          int     `json:"i"`
		J          int     `json:"j"`
		Type       string  `json:"type"`
		Volume     float64 `json:"volume,omitempty"`
		Duration   float64 `json:"duration,omitempty"`
		GrooveKind string  `json:"groove_kind,omitempty"`
		GroovePct  float64 `json:"groove_pct,omitempty"`
	}
	f := struct {
		Version int     `json:"version"`
		Subdiv  int     `json:"subdiv"`
		BPM     int     `json:"bpm"`
		Nodes   []node2 `json:"nodes"`
		Insts   []tInst `json:"instruments"`
	}{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Nodes:   []node2{{ID: 1, I: 0, J: 0, Type: "regular", Volume: -5, Duration: 0, GrooveKind: "weird", GroovePct: 2}},
		Insts:   []tInst{{Name: "Row", ID: "snare", Kind: "builtin", Volume: 9.9, Origin: 1, Color: "#FFFFFFFF"}},
	}
	// Recompose with correct field names for instruments
	type file struct {
		Version     int     `json:"version"`
		Subdiv      int     `json:"subdiv"`
		BPM         int     `json:"bpm"`
		Nodes       []node2 `json:"nodes"`
		Instruments []tInst `json:"instruments"`
	}
	data, _ := json.Marshal(file{Version: f.Version, Subdiv: f.Subdiv, BPM: f.BPM, Nodes: f.Nodes, Instruments: f.Insts})
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if len(g.drum.Rows) == 0 {
		t.Fatalf("no rows after import")
	}
	if !(g.drum.Rows[0].Volume >= 0 && g.drum.Rows[0].Volume <= 4) {
		t.Fatalf("row volume not clamped: %v", g.drum.Rows[0].Volume)
	}
	// Find node param
	var any *uiNode
	for _, n := range g.nodes {
		any = n
		break
	}
	if any == nil {
		t.Fatalf("missing node")
	}
	if mn, ok := g.graph.GetNodeByID(any.ID); ok {
		if mn.Params.Volume < 0 || mn.Params.Volume > 4 {
			t.Fatalf("node volume not clamped: %v", mn.Params.Volume)
		}
		if mn.Params.Duration <= 0 {
			// Duration should not be <=0 after import; default remains 1 (set by model.New node params default)
			t.Fatalf("node duration invalid after import: %v", mn.Params.Duration)
		}
		if mn.Params.GrooveKind != "" {
			t.Fatalf("invalid groove kind should be ignored; got %q", mn.Params.GrooveKind)
		}
	} else {
		t.Fatalf("node not found in graph")
	}
}
