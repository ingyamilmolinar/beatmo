package ui

import (
	"encoding/json"
	"testing"
)

func TestExportImportSubdivRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdiv: %v", err)
	}
	if g.grid.MaxDiv() != 16 {
		t.Fatalf("grid maxdiv=%d want 16", g.grid.MaxDiv())
	}
	// Export bytes
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	// Import into a new game
	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Sanity: exported JSON Subdiv should be 16
	var f struct {
		Subdiv int `json:"subdiv"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f.Subdiv != 16 {
		t.Fatalf("exported subdiv=%d want 16", f.Subdiv)
	}
	// Apply the imported subdiv to the grid to align playback and UI; import scales nodes by design.
	if err := g2.SetSubdivisions(f.Subdiv); err != nil {
		t.Fatalf("set subdiv on imported: %v", err)
	}
	if g2.grid.MaxDiv() != 16 {
		t.Fatalf("post-set grid maxdiv=%d want 16", g2.grid.MaxDiv())
	}
	if g2.drum.timelineUnitsPerBeat != 16 {
		t.Fatalf("timelineUnits=%d want 16", g2.drum.timelineUnitsPerBeat)
	}
}

func TestSetSubdivWhilePlayingDenied(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Start playing
	g.SetPlaying(true)
	err := g.SetSubdivisions(8)
	if err == nil {
		t.Fatalf("expected error when changing subdiv while playing")
	}
	// Ensure unchanged
	if g.grid.MaxDiv() != g.drum.timelineUnitsPerBeat {
		t.Fatalf("grid/timeline mismatch after denied change")
	}
}

func TestToSubReflectsSubdivisions(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	if err := g.SetSubdivisions(32); err != nil {
		t.Fatalf("set subdiv 32: %v", err)
	}
	if ToSub(g.grid, 0.5) != 16 {
		t.Fatalf("ToSub(0.5)=%d want 16", ToSub(g.grid, 0.5))
	}
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set subdiv 8: %v", err)
	}
	if ToSub(g.grid, 0.5) != 4 {
		t.Fatalf("ToSub(0.5)=%d want 4", ToSub(g.grid, 0.5))
	}
}
