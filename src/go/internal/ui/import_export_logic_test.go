package ui

import (
	"encoding/json"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// TestExportIncludesLogicFields ensures new logic fields are present in export
// and that legacy SkipEvery is not emitted.
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
			if n.SkipEvery != 0 {
				t.Fatalf("expected SkipEvery omitted, got %d", n.SkipEvery)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("node not found in export")
	}
}

// TestImportLogicFieldsOverrideSkip verifies import applies logic fields and
// still supports legacy SkipEvery when logic is absent.
func TestImportLogicFieldsOverrideSkip(t *testing.T) {
	assertDefaultParityState(t)
	// Build a minimal export JSON with both logic and skip fields for a node
	file := exportFile{
		Version:     1,
		Subdiv:      32,
		BPM:         120,
		Instruments: []exportInstrument{{Name: "Row", ID: "snare", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF"}},
		Nodes: []exportNode{
			{ID: 1, I: 0, J: 0, Type: "regular", LogicKind: "skip_every_n", LogicN: 2, SkipEvery: 7},
		},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Find node and check it has logic from fields, not legacy skip
	have := false
	for _, n := range g.graph.Nodes {
		if n.I == 0 && n.J == 0 {
			if n.Params.LogicKind != "skip_every_n" || n.Params.LogicN != 2 {
				t.Fatalf("logic not applied: %+v", n.Params)
			}
			// SkipEveryN is deprecated and should be normalized away.
			if n.Params.SkipEveryN != 0 {
				t.Fatalf("unexpected SkipEveryN: %d", n.Params.SkipEveryN)
			}
			have = true
		}
	}
	if !have {
		t.Fatalf("imported node not found")
	}

	// Now import a legacy-only skip file (no logic fields) – importer should
	// upgrade it to logic_kind=skip_every_n so UI reflects it.
	file2 := exportFile{
		Version:     1,
		Subdiv:      32,
		BPM:         120,
		Instruments: []exportInstrument{{Name: "Row", ID: "snare", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF"}},
		Nodes: []exportNode{
			{ID: 1, I: 1, J: 0, Type: "regular", SkipEvery: 4},
		},
	}
	data2, _ := json.Marshal(file2)
	if err := g.Import(data2); err != nil {
		t.Fatalf("import legacy: %v", err)
	}
	have = false
	for _, n := range g.graph.Nodes {
		if n.I == 1 && n.J == 0 {
			if n.Params.LogicKind != "skip_every_n" || n.Params.LogicN != 4 {
				t.Fatalf("legacy skip not upgraded to logic: %+v", n.Params)
			}
			have = true
		}
	}
	if !have {
		t.Fatalf("legacy node not found")
	}
}
