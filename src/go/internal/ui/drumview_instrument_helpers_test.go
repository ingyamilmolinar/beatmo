//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// --- instDisplayLabel tests (preserved from drumview_instrument_menu_test.go) ---

func TestInstDisplayLabel_EmptyID(t *testing.T) {
	dv := &DrumView{}
	got := dv.instDisplayLabel("")
	if got != "(missing)" {
		t.Fatalf("instDisplayLabel(%q) = %q, want %q", "", got, "(missing)")
	}
}

func TestInstDisplayLabel_CacheHit(t *testing.T) {
	dv := &DrumView{
		instLabelCache: map[instrument.ID]string{
			roster.Kick: "Cached Kick",
		},
	}
	got := dv.instDisplayLabel(roster.Kick)
	if got != "Cached Kick" {
		t.Fatalf("instDisplayLabel(%q) = %q, want %q", "kick", got, "Cached Kick")
	}
}

func TestInstDisplayLabel_CacheMiss(t *testing.T) {
	dv := &DrumView{
		instLabelCache: map[instrument.ID]string{},
	}
	got := dv.instDisplayLabel(roster.Snare)
	if got != "Snare" {
		t.Fatalf("instDisplayLabel(%q) = %q, want %q", "snare", got, "Snare")
	}
	cached, ok := dv.instLabelCache[roster.Snare]
	if !ok {
		t.Fatal("expected label to be cached after miss")
	}
	if cached != "Snare" {
		t.Fatalf("cached label = %q, want %q", cached, "Snare")
	}
}

func TestInstDisplayLabel_InitializesCache(t *testing.T) {
	dv := &DrumView{}
	got := dv.instDisplayLabel(roster.Hihat)
	if got != "Hihat" {
		t.Fatalf("instDisplayLabel(%q) = %q, want %q", "hihat", got, "Hihat")
	}
	if dv.instLabelCache == nil {
		t.Fatal("instLabelCache should be initialized after first call")
	}
	if dv.instLabelCache[roster.Hihat] != "Hihat" {
		t.Fatalf("cached = %q, want %q", dv.instLabelCache[roster.Hihat], "Hihat")
	}
}

// Catalog-tier resolution lives in audio.InstrumentDisplayName; these cover the pure-id fallback.

func TestComputeInstLabel_FallbackCapitalize(t *testing.T) {
	dv := &DrumView{}
	got := dv.computeInstLabel(roster.Snare)
	if got != "Snare" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "snare", got, "Snare")
	}
}

func TestComputeInstLabel_SingleChar(t *testing.T) {
	dv := &DrumView{}
	got := dv.computeInstLabel("x")
	if got != "X" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "x", got, "X")
	}
}

func TestComputeInstLabel_MetaEmptyBoth(t *testing.T) {
	dv := &DrumView{
		instMeta: map[instrument.ID]audio.SoundMeta{
			roster.Tom: {},
		},
	}
	got := dv.computeInstLabel(roster.Tom)
	if got != "Tom" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "tom", got, "Tom")
	}
}

// --- matchInstrumentSearch tests ---

func TestMatchInstrumentSearch_EmptyQuery(t *testing.T) {
	dv := &DrumView{}
	if !dv.matchInstrumentSearch(roster.Kick, "") {
		t.Fatal("empty query should always match")
	}
}

func TestMatchInstrumentSearch_MatchID(t *testing.T) {
	dv := &DrumView{}
	if !dv.matchInstrumentSearch("kick808", "kick") {
		t.Fatal("query 'kick' should match id 'kick808'")
	}
}

func TestMatchInstrumentSearch_MatchIDCaseInsensitive(t *testing.T) {
	dv := &DrumView{}
	if !dv.matchInstrumentSearch("HiHat", "hihat") {
		t.Fatal("query 'hihat' should match id 'HiHat' (case-insensitive)")
	}
}

func TestMatchInstrumentSearch_MatchLabel(t *testing.T) {
	// The label resolves through the real catalog, so it is seeded there.
	audio.ResetCatalogForTest([]audio.SoundMeta{{ID: "k808", Name: "808 Deep Kick"}})
	defer audio.ResetCatalogForTest(nil)
	dv := &DrumView{}
	if !dv.matchInstrumentSearch("k808", "deep") {
		t.Fatal("query 'deep' should match label '808 Deep Kick'")
	}
}

func TestMatchInstrumentSearch_MatchRelPath(t *testing.T) {
	dv := &DrumView{
		instMeta: map[instrument.ID]audio.SoundMeta{
			"myinst": {RelPath: "drums/vintage_snare.wav"},
		},
	}
	if !dv.matchInstrumentSearch("myinst", "vintage") {
		t.Fatal("query 'vintage' should match relPath 'drums/vintage_snare.wav'")
	}
}

func TestMatchInstrumentSearch_NoMatch(t *testing.T) {
	dv := &DrumView{}
	if dv.matchInstrumentSearch(roster.Kick, "zzzzz") {
		t.Fatal("query 'zzzzz' should NOT match id 'kick'")
	}
}
