package ui

import (
	"encoding/json"
	"testing"
)

// Test that export includes custom WAV path and import auto-registers it.
func TestExportImportWAVPath(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	// Simulate a custom upload result
	g.drum.pendingWAV = "/tmp/my-kick.wav"
	g.drum.registerInstrument("mykick")
	if !g.drum.IsInstrumentAvailable("mykick") {
		t.Fatalf("expected mykick to be available after register")
	}
	// Set row 0 to custom instrument
	g.drum.Rows[0].Instrument = "mykick"
	g.drum.Rows[0].Name = "MyKick"
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(f.Instruments) == 0 {
		t.Fatalf("missing instruments in export")
	}
	found := false
	for _, inst := range f.Instruments {
		if inst.ID == "mykick" {
			if inst.Path == "" {
				t.Fatalf("expected path in export for custom instrument")
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("custom instrument not found in export")
	}

	// Now import and ensure the instrument becomes available without user upload
	g2 := New(testLogger)
	g2.Layout(640, 480)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	if !g2.drum.IsInstrumentAvailable("mykick") {
		t.Fatalf("custom wav not registered on import")
	}
}
