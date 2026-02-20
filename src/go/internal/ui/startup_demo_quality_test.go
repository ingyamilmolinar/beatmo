package ui

import (
	"encoding/json"
	"testing"

	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
)

// Ensure the embedded startup demo attaches known instruments and avoids
// overlapping regular-node coordinates across different circuits (rows).
func TestStartupDemoQuality(t *testing.T) {
	assertDefaultParityState(t)
	data := assets_pkg.StartupDemoJSON
	if len(data) == 0 {
		t.Fatalf("embedded startup demo JSON missing")
	}

	// Import via Game to build paths and DrumView.
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(g.drum.Rows) < 2 {
		t.Fatalf("expected multiple rows in startup demo; got %d", len(g.drum.Rows))
	}

	// All rows should use available instruments.
	for i, r := range g.drum.Rows {
		if !g.drum.IsInstrumentAvailable(r.Instrument) {
			t.Fatalf("row %d instrument %q not available", i, r.Instrument)
		}
	}

	// Build per-row sets of regular-node coordinates from beat paths and
	// assert rows don't share regular-node coordinates (avoid overlapping
	// circuits in the demo for clearer UX).
	type coord struct{ I, J int }
	coordsByRow := make([]map[coord]bool, len(g.drum.Rows))
	for i := range g.drum.Rows {
		coords := map[coord]bool{}
		for _, bi := range g.beatInfosByRow[i] {
			if bi.NodeType == 0 { // NodeTypeRegular
				coords[coord{bi.I, bi.J}] = true
			}
		}
		coordsByRow[i] = coords
	}
	for i := 0; i < len(coordsByRow); i++ {
		for j := i + 1; j < len(coordsByRow); j++ {
			for c := range coordsByRow[i] {
				if coordsByRow[j][c] {
					t.Fatalf("rows %d and %d share regular coordinate (%d,%d)", i, j, c.I, c.J)
				}
			}
		}
	}
}

// Ensure the startup demo includes at least one node using built-in logic to
// showcase features like probability or every-n.
func TestStartupDemoIncludesLogicNodes(t *testing.T) {
	assertDefaultParityState(t)
	data := assets_pkg.StartupDemoJSON
	if len(data) == 0 {
		t.Fatalf("embedded startup demo JSON missing")
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hasLogic := false
	for _, n := range f.Nodes {
		if n.LogicKind != "" || n.SkipEvery != 0 || n.Pitch != 0 || n.Duration != 0 {
			hasLogic = true
			break
		}
	}
	if !hasLogic {
		t.Fatalf("expected at least one node with logic or params in startup demo JSON")
	}
}

// Ensure the startup demo includes EQ configuration (a key production feature).
func TestStartupDemoIncludesEQ(t *testing.T) {
	assertDefaultParityState(t)
	data := assets_pkg.StartupDemoJSON
	if len(data) == 0 {
		t.Fatalf("embedded startup demo JSON missing")
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Check that at least one instrument has non-default EQ
	hasInstrumentEQ := false
	for _, inst := range f.Instruments {
		if inst.EQ != nil && len(inst.EQ.GainsDB) > 0 {
			hasInstrumentEQ = true
			break
		}
	}
	if !hasInstrumentEQ {
		t.Fatalf("expected at least one instrument with EQ configuration in startup demo")
	}

	// Check master EQ
	if f.EQ == nil || len(f.EQ.GainsDB) == 0 {
		t.Fatalf("expected master EQ configuration in startup demo")
	}
}

// Verifies that the embedded startup demo JSON populates preview.
func TestEmbeddedStartupDemoPopulatesPreview(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	data := assets_pkg.StartupDemoJSON
	if len(data) == 0 {
		t.Fatalf("embedded startup demo JSON missing")
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
		t.Fatalf("embedded startup demo produced no visible steps in preview")
	}
}
