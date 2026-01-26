//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestRegisterInstrumentNewDoesNotChangeSelection(t *testing.T) {
	withDefaultAudio(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	initial := g.drum.Rows[0].Instrument
	startUploadForTest(t, g.drum)
	waitForUploadNaming(t, g)
	g.drum.registerInstrument("custom_new")
	if g.drum.Rows[0].Instrument != initial {
		t.Fatalf("expected selected row to keep instrument %q, got %q", initial, g.drum.Rows[0].Instrument)
	}
	found := false
	for _, id := range g.drum.instOptions {
		if id == "custom_new" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected custom_new to be available in instrument list")
	}
}

func TestRegisterInstrumentReplacesMatchingRowsOnly(t *testing.T) {
	withDefaultAudio(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	if len(g.drum.Rows) == 0 {
		t.Fatalf("expected default drum rows")
	}
	g.drum.Update()
	if len(g.drum.rowLabels) < 1 {
		t.Fatalf("expected row labels for selection")
	}
	g.drum.rowLabels[0].OnClick()
	g.drum.SetInstrument("kick")
	g.drum.AddRow()
	g.drum.Update()
	if len(g.drum.rowLabels) < 2 {
		t.Fatalf("expected row labels for selection")
	}
	g.drum.rowLabels[1].OnClick()
	g.drum.SetInstrument("snare")
	g.drum.rowLabels[0].OnClick() // selection unrelated to the instrument being updated

	startUploadForTest(t, g.drum)
	waitForUploadNaming(t, g)
	g.drum.registerInstrument("snare")

	if g.drum.Rows[0].Instrument != "kick" {
		t.Fatalf("row 0 instrument replaced unexpectedly: got %q", g.drum.Rows[0].Instrument)
	}
	if g.drum.Rows[1].Instrument != "snare" {
		t.Fatalf("row 1 instrument changed: got %q", g.drum.Rows[1].Instrument)
	}
}
