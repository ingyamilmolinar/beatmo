//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
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
		instLabelCache: map[string]string{
			"kick": "Cached Kick",
		},
	}
	got := dv.instDisplayLabel("kick")
	if got != "Cached Kick" {
		t.Fatalf("instDisplayLabel(%q) = %q, want %q", "kick", got, "Cached Kick")
	}
}

func TestInstDisplayLabel_CacheMiss(t *testing.T) {
	dv := &DrumView{
		instLabelCache: map[string]string{},
	}
	got := dv.instDisplayLabel("snare")
	if got != "Snare" {
		t.Fatalf("instDisplayLabel(%q) = %q, want %q", "snare", got, "Snare")
	}
	cached, ok := dv.instLabelCache["snare"]
	if !ok {
		t.Fatal("expected label to be cached after miss")
	}
	if cached != "Snare" {
		t.Fatalf("cached label = %q, want %q", cached, "Snare")
	}
}

func TestInstDisplayLabel_InitializesCache(t *testing.T) {
	dv := &DrumView{}
	got := dv.instDisplayLabel("hihat")
	if got != "Hihat" {
		t.Fatalf("instDisplayLabel(%q) = %q, want %q", "hihat", got, "Hihat")
	}
	if dv.instLabelCache == nil {
		t.Fatal("instLabelCache should be initialized after first call")
	}
	if dv.instLabelCache["hihat"] != "Hihat" {
		t.Fatalf("cached = %q, want %q", dv.instLabelCache["hihat"], "Hihat")
	}
}

// --- computeInstLabel tests ---

func TestComputeInstLabel_WithMetaName(t *testing.T) {
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"kick808": {Name: "808 Kick"},
		},
	}
	got := dv.computeInstLabel("kick808")
	if got != "808 Kick" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "kick808", got, "808 Kick")
	}
}

func TestComputeInstLabel_WithMetaRelPath(t *testing.T) {
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"myinst": {RelPath: "drums/deep_kick.wav"},
		},
	}
	got := dv.computeInstLabel("myinst")
	if got != "Deep Kick" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "myinst", got, "Deep Kick")
	}
}

func TestComputeInstLabel_FallbackCapitalize(t *testing.T) {
	dv := &DrumView{}
	got := dv.computeInstLabel("snare")
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

func TestComputeInstLabel_MetaEmptyNameUsesRelPath(t *testing.T) {
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"test": {Name: "", RelPath: "samples/bright_snare.wav"},
		},
	}
	got := dv.computeInstLabel("test")
	if got != "Bright Snare" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "test", got, "Bright Snare")
	}
}

func TestComputeInstLabel_MetaEmptyBoth(t *testing.T) {
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"tom": {},
		},
	}
	got := dv.computeInstLabel("tom")
	if got != "Tom" {
		t.Fatalf("computeInstLabel(%q) = %q, want %q", "tom", got, "Tom")
	}
}

// --- matchInstrumentSearch tests ---

func TestMatchInstrumentSearch_EmptyQuery(t *testing.T) {
	dv := &DrumView{}
	if !dv.matchInstrumentSearch("kick", "") {
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
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"k808": {Name: "808 Deep Kick"},
		},
	}
	if !dv.matchInstrumentSearch("k808", "deep") {
		t.Fatal("query 'deep' should match label '808 Deep Kick'")
	}
}

func TestMatchInstrumentSearch_MatchRelPath(t *testing.T) {
	dv := &DrumView{
		instMeta: map[string]audio.SoundMeta{
			"myinst": {RelPath: "drums/vintage_snare.wav"},
		},
	}
	if !dv.matchInstrumentSearch("myinst", "vintage") {
		t.Fatal("query 'vintage' should match relPath 'drums/vintage_snare.wav'")
	}
}

func TestMatchInstrumentSearch_NoMatch(t *testing.T) {
	dv := &DrumView{}
	if dv.matchInstrumentSearch("kick", "zzzzz") {
		t.Fatal("query 'zzzzz' should NOT match id 'kick'")
	}
}
