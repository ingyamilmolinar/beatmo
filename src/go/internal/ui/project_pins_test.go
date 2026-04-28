package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestProjectPins_JSONRoundTrip pins the contract that PinnedInstruments
// survives an export → import cycle without reordering or duplication.
// Project pins live alongside the graph in the project JSON; user favorites
// (the ★ store) live in userprefs and never round-trip through this file.
func TestProjectPins_JSONRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui

	// Seed pins on the source DrumView. SetProjectPins dedupes empty strings;
	// the helper itself sorts on export, so the JSON will hold them sorted.
	g.drum.SetProjectPins([]string{"kick-808", "snare-tight", "hh-shuffle"})

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	gotSorted := append([]string(nil), f.PinnedInstruments...)
	wantSorted := []string{"hh-shuffle", "kick-808", "snare-tight"}
	if len(gotSorted) != len(wantSorted) {
		t.Fatalf("pinned_instruments len: got %d want %d (%v)", len(gotSorted), len(wantSorted), gotSorted)
	}
	for i := range wantSorted {
		if gotSorted[i] != wantSorted[i] {
			t.Errorf("pinned_instruments[%d]: got %q want %q (full %v)", i, gotSorted[i], wantSorted[i], gotSorted)
		}
	}

	// Import back into a fresh Game and assert the pin set survives.
	g2 := New(logger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(640, 480)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	pins := g2.drum.ProjectPins()
	if len(pins) != 3 {
		t.Fatalf("imported pin set size: got %d want 3 (%v)", len(pins), pins)
	}
	for _, id := range []string{"kick-808", "snare-tight", "hh-shuffle"} {
		if _, ok := pins[id]; !ok {
			t.Errorf("imported pin set missing %q (full %v)", id, pins)
		}
	}
}

// TestProjectPins_OmitWhenEmpty pins the omitempty contract on
// `pinned_instruments`: an empty / nil pin set must produce an absent JSON
// field so projects authored before this PR remain byte-identical when
// re-exported.
func TestProjectPins_OmitWhenEmpty(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	// Default state: no pins.

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if strings.Contains(string(data), `"pinned_instruments"`) {
		t.Errorf("expected pinned_instruments to be omitted when empty; got JSON: %s", data)
	}
}

// TestProjectPins_BackwardCompatible pins the contract that v1 projects
// without the field still parse cleanly and produce an empty pin set.
// Hand-rolled JSON to avoid depending on the export path's other fields.
func TestProjectPins_BackwardCompatible(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Seed with pins so we can prove import resets them when the field is
	// absent (avoids the "pin set silently inherited from previous project"
	// failure mode).
	g.drum.SetProjectPins([]string{"will-be-cleared"})

	legacy := `{"version":1,"subdiv":32,"bpm":120,"instruments":[],"nodes":[]}`
	if err := g.Import([]byte(legacy)); err != nil {
		t.Fatalf("legacy import: %v", err)
	}
	if pins := g.drum.ProjectPins(); pins != nil {
		t.Errorf("legacy import should reset pins; got %v", pins)
	}
}

// TestProjectPins_SetProjectPinsDedupesEmpty pins that empty strings in the
// input slice are filtered. Future code paths (manual edits to project JSON,
// deserialised pinned_instruments arrays with placeholder entries) shouldn't
// be able to inject zero-id pins that would otherwise match every menu row
// failing PinSource.Tier lookup.
func TestProjectPins_SetProjectPinsDedupesEmpty(t *testing.T) {
	dv := &DrumView{}
	dv.SetProjectPins([]string{"kick-808", "", "snare-tight", ""})
	pins := dv.ProjectPins()
	if len(pins) != 2 {
		t.Fatalf("expected 2 pins after empty-string filter; got %d (%v)", len(pins), pins)
	}
	if _, ok := pins[""]; ok {
		t.Errorf("empty-string id leaked into pin set (%v)", pins)
	}
}
