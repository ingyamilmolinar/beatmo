package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestImportRejectsUnknownDocumentValue: a project whose instrument document
// names a value the engine does not know (post_enabled, retired in Phase C) is
// rejected with the name and its path — never imported with the key silently
// dropped — and the current circuit is untouched.
func TestImportRejectsUnknownDocumentValue(t *testing.T) {
	g := newDocsGame(t, roster.Snare)
	before := mustExport(t, g)
	var f map[string]any
	if err := json.Unmarshal(before, &f); err != nil {
		t.Fatal(err)
	}
	doc := f["instrument_docs"].(map[string]any)["snare"].(map[string]any)
	doc["stages"].(map[string]any)["core"].(map[string]any)["values"].(map[string]any)["post_enabled"] = 1
	bad, _ := json.Marshal(f)
	err := g.Import(bad)
	if err == nil || !strings.Contains(err.Error(), "post_enabled") || !strings.Contains(err.Error(), "instrument_docs.snare") {
		t.Errorf("err = %v, want a rejection naming instrument_docs.snare and post_enabled", err)
	}
	if after := mustExport(t, g); string(after) != string(before) {
		t.Error("a rejected import changed the current circuit")
	}
}

// TestBundleImportValidatesEveryDocFirst: a bundle whose SECOND instrument is on
// a dead base registers NOTHING — the first doc used to be registered, bound and
// persisted before the second one failed.
func TestBundleImportValidatesEveryDocFirst(t *testing.T) {
	g := newDocsGame(t, roster.Snare)
	snare, _ := audio.ShippedDoc(roster.Snare)
	good := snare.Clone()
	good.ID, good.Name = "user.snare.good01", "Good"
	bad := snare.Clone()
	bad.ID, bad.Name = "user.snare.bad01", "Bad"
	bad.FX = append(bad.FX, instrument.Module{Type: "fx.warp", V: 1, On: true, Values: instrument.Values{}})

	b := audio.NewInstrumentBundle()
	b.Instruments = []audio.BundleInstrument{{Name: "Good", Synth: &good}, {Name: "Bad", Synth: &bad}}
	if _, err := g.drum.ApplyInstrumentBundle(&b, nil, -1, nil); err == nil || !strings.Contains(err.Error(), "fx.warp") {
		t.Fatalf("bundle with an unknown module type: err = %v, want a rejection naming it", err)
	}
	if _, ok := audio.LibraryDoc("user.snare.good01"); ok {
		t.Error("the first instrument was saved before the bundle was rejected")
	}
	if _, ok := audio.InstrumentDoc("user.snare.good01"); ok {
		t.Error("the first instrument was registered before the bundle was rejected")
	}

	// Same contract for a known module carrying a value the engine does not have.
	bad2 := snare.Clone()
	bad2.ID, bad2.Name = "user.snare.bad02", "Bad"
	core := bad2.Stages["core"]
	core.Values["post_tone_on"] = 1
	bad2.Stages["core"] = core
	b.Instruments[1] = audio.BundleInstrument{Name: "Bad", Synth: &bad2}
	if _, err := g.drum.ApplyInstrumentBundle(&b, nil, -1, nil); err == nil || !strings.Contains(err.Error(), "post_tone_on") {
		t.Fatalf("bundle with an unknown value: err = %v, want a rejection naming it", err)
	}
	if _, ok := audio.LibraryDoc("user.snare.good01"); ok {
		t.Error("the first instrument was saved before the unknown-value bundle was rejected")
	}
}
