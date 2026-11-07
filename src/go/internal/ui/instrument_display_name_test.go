//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// With no catalogue entry "hi-hat" renders "Hi Hat".
func TestComputeInstLabel_HyphenatedIDFallback(t *testing.T) {
	dv := &DrumView{}
	got := dv.computeInstLabel("hi-hat")
	if got != "Hi Hat" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "hi-hat", got, "Hi Hat")
	}
}

// TestComputeInstLabel_UnderscoredIDFallback covers the other separator.
func TestComputeInstLabel_UnderscoredIDFallback(t *testing.T) {
	dv := &DrumView{}
	got := dv.computeInstLabel("open_hi_hat")
	if got != "Open Hi Hat" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "open_hi_hat", got, "Open Hi Hat")
	}
}

func newTestDrumViewForName(t *testing.T) *DrumView {
	t.Helper()
	logger := game_log.New(testLogOutput(), game_log.LevelDebug)
	graph := model.NewGraph(logger)
	return NewDrumView(image.Rect(0, 0, 200, 200), graph, logger)
}

// A builtin id with no separators, so roster changes don't matter: this checks the wiring.
func TestNewDrumView_RowNameUsesPrettyName(t *testing.T) {
	dv := newTestDrumViewForName(t)
	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	want := audio.PrettyName(string(dv.Rows[0].Instrument))
	if dv.Rows[0].Name != want {
		t.Fatalf("Rows[0].Name = %q, want %q (audio.PrettyName(%q))",
			dv.Rows[0].Name, want, dv.Rows[0].Instrument)
	}
}

// "hi-hat" bites if AddRow falls back to an inline title-case.
func TestAddRow_RowNameUsesComputeInstLabel(t *testing.T) {
	dv := newTestDrumViewForName(t)
	dv.instOptions = []instrument.ID{"hi-hat"}
	before := len(dv.Rows)
	dv.AddRow()
	if len(dv.Rows) != before+1 {
		t.Fatalf("AddRow did not append: rows=%d, want %d", len(dv.Rows), before+1)
	}
	got := dv.Rows[len(dv.Rows)-1].Name
	if got != "Hi Hat" {
		t.Fatalf("AddRow Rows[last].Name = %q, want %q", got, "Hi Hat")
	}
}

func TestSetInstrument_RowNameFromCatalogName(t *testing.T) {
	dv := newTestDrumViewForName(t)
	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	dv.instMeta = map[instrument.ID]audio.SoundMeta{
		"hi-hat": {ID: "hi-hat", Name: "Hi Hat"},
	}
	// Bust the label cache so the next computeInstLabel sees instMeta.
	dv.instLabelCache = nil
	dv.selRow = 0
	dv.SetInstrument("hi-hat")
	if dv.Rows[0].Instrument != "hi-hat" {
		t.Fatalf("Rows[0].Instrument = %q, want %q", dv.Rows[0].Instrument, "hi-hat")
	}
	if dv.Rows[0].Name != "Hi Hat" {
		t.Fatalf("Rows[0].Name = %q, want %q", dv.Rows[0].Name, "Hi Hat")
	}
}

func TestSetInstrument_RowNameFallsBackToPrettyName(t *testing.T) {
	dv := newTestDrumViewForName(t)
	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	dv.instMeta = nil
	dv.instLabelCache = nil
	dv.selRow = 0
	dv.SetInstrument("hi-hat")
	if dv.Rows[0].Name != "Hi Hat" {
		t.Fatalf("Rows[0].Name = %q, want %q", dv.Rows[0].Name, "Hi Hat")
	}
}
