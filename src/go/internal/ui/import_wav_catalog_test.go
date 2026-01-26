package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

// TestExportWAVInstrumentIncludesKindAndPath verifies that WAV-based instruments
// are exported with kind="sample" and include their path.
func TestExportWAVInstrumentIncludesKindAndPath(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Set up catalog with a WAV instrument that doesn't have "sample-" prefix
	// AFTER New() to prevent InitDefaultCatalog from overwriting it
	audio.ResetCatalogForTest([]audio.SoundMeta{
		{
			ID:       "kick-goldbaby-bd-test",
			Name:     "Kick Goldbaby",
			Category: "Kick Drums (WAV)",
			Source:   "wav",
			Path:     "/fake/path/to/kick.wav",
		},
	})
	t.Cleanup(func() { audio.ResetCatalogForTest(nil) })

	// Clear default rows and set up a single row with the WAV instrument
	g.drum.Rows = nil
	g.drum.AddRow()
	g.drum.Rows[0].Instrument = "kick-goldbaby-bd-test"
	g.drum.Rows[0].Name = "Kick"

	// Export
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(f.Instruments) != 1 {
		t.Fatalf("expected 1 instrument, got %d", len(f.Instruments))
	}

	inst := f.Instruments[0]
	if inst.Kind != "sample" {
		t.Errorf("expected kind=sample for WAV instrument, got %q", inst.Kind)
	}
	if inst.Path != "/fake/path/to/kick.wav" {
		t.Errorf("expected path to be included, got %q", inst.Path)
	}
}

// TestImportWAVInstrumentFromCatalog verifies that importing a project with
// WAV instruments loads them via catalog lookup.
func TestImportWAVInstrumentFromCatalog(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Set up catalog with WAV instrument AFTER New() to prevent overwrite
	audio.ResetCatalogForTest([]audio.SoundMeta{
		{
			ID:       "kick-kick-goldbaby-bd-test",
			Name:     "Kick Goldbaby",
			Category: "Kick Drums (WAV)",
			Source:   "wav",
			Path:     "/fake/path/to/kick.wav",
		},
	})
	t.Cleanup(func() { audio.ResetCatalogForTest(nil) })

	// Import with builtin kind (simulates old export format)
	f := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Instruments: []exportInstrument{
			{
				Name:   "Kick",
				ID:     "kick-kick-goldbaby-bd-test",
				Kind:   "builtin", // Bug: old exports have "builtin"
				Volume: 1,
				Origin: 0,
				Color:  "#FFFFFFFF",
			},
		},
		Nodes: []exportNode{
			{ID: 0, I: 0, J: 0, Type: "regular"},
		},
	}
	data, _ := json.Marshal(f)

	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Verify instrument is available (loaded from catalog)
	if !g.drum.IsInstrumentAvailable("kick-kick-goldbaby-bd-test") {
		t.Fatalf("instrument should be available after import from catalog")
	}

	// Verify row label does NOT have MissingInstStyle
	g.drum.calcLayout()
	if len(g.drum.rowLabels) == 0 {
		t.Fatalf("no row labels")
	}
	style, ok := g.drum.rowLabels[0].Style.(ButtonStyle)
	if !ok {
		t.Fatalf("label style is not a ButtonStyle")
	}
	if style == MissingInstStyle {
		t.Errorf("row label should not have MissingInstStyle for available instrument")
	}
}

// TestImportMissingWAVShowsRedLabel verifies missing instruments show red labels.
func TestImportMissingWAVShowsRedLabel(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Empty catalog - no instruments available (after New() to prevent overwrite)
	audio.ResetCatalogForTest(nil)
	t.Cleanup(func() { audio.ResetCatalogForTest(nil) })

	f := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Instruments: []exportInstrument{
			{
				Name:   "Missing Kick",
				ID:     "nonexistent-kick-wav",
				Kind:   "sample",
				Volume: 1,
				Origin: 0,
				Color:  "#FFFFFFFF",
			},
		},
		Nodes: []exportNode{
			{ID: 0, I: 0, J: 0, Type: "regular"},
		},
	}
	data, _ := json.Marshal(f)

	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Verify instrument is NOT available
	if g.drum.IsInstrumentAvailable("nonexistent-kick-wav") {
		t.Fatalf("instrument should not be available")
	}

	// Verify row label HAS MissingInstStyle
	g.drum.calcLayout()
	if len(g.drum.rowLabels) == 0 {
		t.Fatalf("no row labels")
	}
	style, ok := g.drum.rowLabels[0].Style.(ButtonStyle)
	if !ok {
		t.Fatalf("label style is not a ButtonStyle")
	}
	if style != MissingInstStyle {
		t.Errorf("row label should have MissingInstStyle for missing instrument")
	}
}
