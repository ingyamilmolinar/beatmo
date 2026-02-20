package ui

import (
	"encoding/json"
	"testing"

	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
)

// Verifies that importing a simple loop populates drum preview steps.
func TestImportPopulatesDrumPreview_SimpleLoop(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	// Simple two-node loop at 0 and 16
	exp := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     110,
		Nodes: []exportNode{
			{ID: 0, I: 0, J: 0, Type: "regular", Outputs: []int{1}},
			{ID: 1, I: 16, J: 0, Type: "regular", Outputs: []int{0}},
		},
		Instruments: []exportInstrument{{Name: "Row", ID: "snare", Kind: "builtin", Volume: 1, Origin: 0, Color: "#FFFFFFFF"}},
	}
	data, _ := json.Marshal(exp)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	// After import, preview should have at least one true cell within window.
	if len(g.drum.Rows) == 0 {
		t.Fatalf("no rows")
	}
	g.refreshDrumRow()
	steps := g.drum.Rows[0].Steps
	if len(steps) == 0 {
		t.Fatalf("no steps computed")
	}
	any := false
	for _, v := range steps {
		if v {
			any = true
			break
		}
	}
	if !any {
		t.Fatalf("expected at least one visible step in preview; got none")
	}
}

// Verifies that the embedded test fixture demo JSON also populates preview.
func TestEmbeddedTestFixtureDemoPopulatesPreview(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	data := assets_pkg.TestFixtureDemoJSON
	if len(data) == 0 {
		t.Fatalf("embedded demo JSON missing")
	}
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	g.refreshDrumRow()
	if len(g.drum.Rows) == 0 {
		t.Fatalf("no rows")
	}
	steps := g.drum.Rows[0].Steps
	if len(steps) == 0 {
		t.Fatalf("no steps computed")
	}
	any := false
	for _, v := range steps {
		if v {
			any = true
			break
		}
	}
	if !any {
		t.Fatalf("embedded demo produced no visible steps in preview")
	}
}
