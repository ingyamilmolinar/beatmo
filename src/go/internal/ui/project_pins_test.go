package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// Pins live in the project; the ★ favourites live in userprefs and never travel in this file.
func TestProjectPins_JSONRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui

	// Export sorts them, so the JSON holds them sorted.
	g.drum.SetProjectPins([]instrument.ID{roster.Kick808, roster.SnareFat, roster.HihatPedal})

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f projectFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	gotSorted := append([]instrument.ID(nil), f.PinnedInstruments...)
	wantSorted := []instrument.ID{roster.HihatPedal, roster.Kick808, roster.SnareFat}
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
	for _, id := range []instrument.ID{roster.Kick808, roster.SnareFat, roster.HihatPedal} {
		if _, ok := pins[id]; !ok {
			t.Errorf("imported pin set missing %q (full %v)", id, pins)
		}
	}
}

// An empty set leaves the field absent.
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

func TestProjectPins_BackwardCompatible(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Seeded so the test proves a file without the field resets the pins instead of inheriting them.
	g.drum.SetProjectPins([]instrument.ID{"will-be-cleared"})

	legacy := `{"schema_version":"` + schema.Version + `","subdiv":32,"bpm":120,"instruments":[],"nodes":[]}`
	if err := g.Import([]byte(legacy)); err != nil {
		t.Fatalf("legacy import: %v", err)
	}
	if pins := g.drum.ProjectPins(); pins != nil {
		t.Errorf("legacy import should reset pins; got %v", pins)
	}
}

// An empty id would match every menu row that fails PinSource.Tier.
func TestProjectPins_SetProjectPinsDedupesEmpty(t *testing.T) {
	dv := &DrumView{}
	dv.SetProjectPins([]instrument.ID{roster.Kick808, "", roster.SnareFat, ""})
	pins := dv.ProjectPins()
	if len(pins) != 2 {
		t.Fatalf("expected 2 pins after empty-string filter; got %d (%v)", len(pins), pins)
	}
	if _, ok := pins[""]; ok {
		t.Errorf("empty-string id leaked into pin set (%v)", pins)
	}
}
