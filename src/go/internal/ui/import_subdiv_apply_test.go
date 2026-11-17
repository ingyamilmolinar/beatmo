package ui

import (
	"encoding/json"
	"testing"
)

// Ensure that importing a JSON with a smaller subdivision applies that
// subdivision to the game's grid and drum timeline without requiring a manual
// SetSubdivisions call.
func TestImportAppliesSubdivToGrid(t *testing.T) {
	assertDefaultParityState(t)
	// Build and export a simple project at 8 subdivisions per beat.
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set subdiv(8): %v", err)
	}
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f struct {
		Subdiv int `json:"subdiv"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f.Subdiv != 8 {
		t.Fatalf("exported subdiv=%d want 8", f.Subdiv)
	}

	// Import into a fresh game with default grid (32). Import should update
	// the game's subdivision to 8 automatically.
	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if g2.grid.MaxDiv() != 32 {
		t.Fatalf("pre import maxdiv=%d want 32", g2.grid.MaxDiv())
	}
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	if g2.grid.MaxDiv() != 8 {
		t.Fatalf("post import grid maxdiv=%d want 8", g2.grid.MaxDiv())
	}
	if g2.drum.timelineUnitsPerBeat != 8 {
		t.Fatalf("timeline units=%d want 8", g2.drum.timelineUnitsPerBeat)
	}
}
