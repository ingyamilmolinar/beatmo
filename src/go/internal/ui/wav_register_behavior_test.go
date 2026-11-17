//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
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
	custom := wavInstrumentID("custom_new")
	if g.drum.Rows[0].Instrument != initial {
		t.Fatalf("expected selected row to keep instrument %q, got %q", initial, g.drum.Rows[0].Instrument)
	}
	found := false
	for _, id := range g.drum.instOptions {
		if id == custom {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected custom_new to be available in instrument list")
	}
}

// TestWAVNamedAfterABuiltinIsItsOwnInstrument: a WAV the user names "Snare"
// becomes a new user-sample instrument labelled "Snare". It never takes the
// built-in snare's id, so the rows playing the snare keep the synth, and the
// built-in stays in the menu beside it. (Before, the typed name WAS the id:
// "Snare" matched "snare" and the WAV replaced the shipped snare.)
func TestWAVNamedAfterABuiltinIsItsOwnInstrument(t *testing.T) {
	withDefaultAudio(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	g.drum.Update()
	g.drum.rowLabels()[0].OnClick()
	g.drum.SetInstrument(roster.Snare)

	startUploadForTest(t, g.drum)
	waitForUploadNaming(t, g)
	g.drum.registerInstrument("Snare")

	wav := wavInstrumentID("Snare")
	if wav == roster.Snare || !wav.IsUser() {
		t.Fatalf("the WAV got id %q; it must be a user sample, never the built-in", wav)
	}
	if g.drum.Rows[0].Instrument != roster.Snare {
		t.Fatalf("row 0 instrument changed: got %q", g.drum.Rows[0].Instrument)
	}
	if !g.drum.IsInstrumentAvailable(wav) || !g.drum.IsInstrumentAvailable(roster.Snare) {
		t.Fatal("both the WAV and the built-in snare must stay available")
	}
	if got := audio.InstrumentDisplayName(wav); got != "Snare" {
		t.Fatalf("WAV display name = %q, want the typed name", got)
	}
	if got := audio.InstrumentDisplayName(roster.Snare); got == "" {
		t.Fatal("the built-in lost its name")
	}

	// Loading another file under the same name updates that WAV instrument.
	startUploadForTest(t, g.drum)
	waitForUploadNaming(t, g)
	g.drum.registerInstrument("snare")
	if wavInstrumentID("snare") != wav {
		t.Fatal("the same name (any case) must name the same WAV instrument")
	}
}
