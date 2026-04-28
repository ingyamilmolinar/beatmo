//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestComputeInstLabel_HyphenatedIDFallback pins the regression target:
// when the catalog has no entry, an id like "hi-hat" must render as
// "Hi Hat" — not the previous broken "Hi-hat".
func TestComputeInstLabel_HyphenatedIDFallback(t *testing.T) {
	dv := &DrumView{}
	got := dv.computeInstLabel("hi-hat")
	if got != "Hi Hat" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "hi-hat", got, "Hi Hat")
	}
}

// TestComputeInstLabel_HyphenatedRelPathFallback pins the regression
// target for the RelPath fallback path: a sample at "x/hi-hat.wav"
// with no explicit Name must render as "Hi Hat".
func TestComputeInstLabel_HyphenatedRelPathFallback(t *testing.T) {
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"x": {RelPath: "samples/hi-hat.wav"},
		},
	}
	got := dv.computeInstLabel("x")
	if got != "Hi Hat" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "x", got, "Hi Hat")
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

// TestNewDrumView_RowNameUsesPrettyName ensures the initial row's
// Name flows through audio.PrettyName, not an inline title-case.
// We pass through a known builtin id (no separators) so any builtin
// rotation still passes — the goal is the wiring, not a specific id.
func TestNewDrumView_RowNameUsesPrettyName(t *testing.T) {
	dv := newTestDrumViewForName(t)
	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	want := audio.PrettyName(dv.Rows[0].Instrument)
	if dv.Rows[0].Name != want {
		t.Fatalf("Rows[0].Name = %q, want %q (audio.PrettyName(%q))",
			dv.Rows[0].Name, want, dv.Rows[0].Instrument)
	}
}

// TestAddRow_RowNameUsesComputeInstLabel covers the runtime AddRow
// path. Override instOptions[0] to "hi-hat" so the regression bites
// if AddRow falls back to the inline title-case.
func TestAddRow_RowNameUsesComputeInstLabel(t *testing.T) {
	dv := newTestDrumViewForName(t)
	dv.instOptions = []string{"hi-hat"}
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

// TestSetInstrument_RowNameUsesComputeInstLabel covers the dropdown
// path. Seed instMeta so computeInstLabel finds a catalog entry, and
// verify SetInstrument writes that entry's Name to Rows[selRow].Name.
func TestSetInstrument_RowNameFromCatalogName(t *testing.T) {
	dv := newTestDrumViewForName(t)
	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	dv.instMeta = map[string]audio.SoundMeta{
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

// TestSetInstrument_RowNameFallsBackToPrettyName covers the fallback
// path: when instMeta has no entry for the id, SetInstrument must
// still produce a properly pretty-cased name (not "Hi-hat").
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
