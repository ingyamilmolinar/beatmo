package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestExportIncludesLogicFields ensures logic fields are present in export.
func TestExportIncludesLogicFields(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	// Use skip_every_n logic
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 3
		g.graph.SetNodeParams(a.ID, p)
	}
	// Hook row origin so export contains the node
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	found := false
	for _, n := range f.Nodes {
		if n.I == 0 && n.J == 0 {
			if n.LogicKind != "skip_every_n" || n.LogicN != 3 {
				t.Fatalf("missing logic fields: kind=%q n=%d", n.LogicKind, n.LogicN)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("node not found in export")
	}
}

// TestImportLogicFields verifies import applies the logic fields onto node params.
func TestImportLogicFields(t *testing.T) {
	assertDefaultParityState(t)
	t.Cleanup(func() { audio.ClearInstrumentDisplayName("snare") })
	file := exportFile{
		Version:     1,
		Subdiv:      32,
		BPM:         120,
		Instruments: []exportInstrument{{Name: "Row", ID: "snare", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF"}},
		Nodes: []exportNode{
			{ID: 1, I: 0, J: 0, Type: "regular", LogicKind: "skip_every_n", LogicN: 2},
		},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	have := false
	for _, n := range g.graph.Nodes {
		if n.I == 0 && n.J == 0 {
			if n.Params.LogicKind != "skip_every_n" || n.Params.LogicN != 2 {
				t.Fatalf("logic not applied: %+v", n.Params)
			}
			have = true
		}
	}
	if !have {
		t.Fatalf("imported node not found")
	}
}

func TestImportSetsDisplayNameOverride(t *testing.T) {
	assertDefaultParityState(t)
	audio.ClearInstrumentDisplayName("kick")
	t.Cleanup(func() { audio.ClearInstrumentDisplayName("kick") })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	js := `{"version":1,"subdiv":8,"bpm":120,` +
		`"instruments":[{"name":"Boomer","id":"kick","kind":"builtin","volume":1,"origin":0,"color":"#C87850FF"}],` +
		`"nodes":[{"id":0,"i":0,"j":0,"type":"regular","outputs":[]}]}`
	if err := g.Import([]byte(js)); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := audio.InstrumentDisplayName("kick"); got != "Boomer" {
		t.Fatalf("override not set on import: got %q", got)
	}
}
