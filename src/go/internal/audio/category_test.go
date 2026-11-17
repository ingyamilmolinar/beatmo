package audio

import (
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestCategoryOfBuiltins(t *testing.T) {
	cases := map[string]Category{
		string(roster.Kick):           CatKick,
		string(roster.Snare):          CatSnare,
		string(roster.Hihat):          CatHiHat,
		string(roster.Violin):         CatStrings,
		string(roster.GuitarElectric): CatGuitar,
		string(roster.Organ):          CatKeys,
		string(roster.Trumpet):        CatBrass,
		string(roster.Flute):          CatWoodwind,
		"totally-unknown-xyz":         CatOther,
	}
	for id, want := range cases {
		if got := CategoryOf(instrument.ID(id)); got != want {
			t.Errorf("CategoryOf(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestCategoryByTextHatNotOverbroad(t *testing.T) {
	// Bare "hat" Contains would misclassify unrelated ids; the rule is a suffix.
	if got, ok := categoryByText("ratchet"); ok {
		t.Errorf("categoryByText(%q) = (%q,true), want no match", "ratchet", got)
	}
	// A genuine open-hat kit piece (suffix "hat") still classifies as HiHat.
	if got, ok := categoryByText("open-hat"); !ok || got != CatHiHat {
		t.Errorf("categoryByText(%q) = (%q,%v), want (CatHiHat,true)", "open-hat", got, ok)
	}
}

func TestInstrumentsByCategoryContainsKick(t *testing.T) {
	got := InstrumentsByCategory(CatKick)
	if !slices.Contains(got, roster.Kick) {
		t.Fatalf("InstrumentsByCategory(CatKick) missing 'kick': %v", got)
	}
}

func TestVoiceCategoryClassification(t *testing.T) {
	for _, id := range []instrument.ID{roster.VoiceSoprano, roster.VoiceWhisper, roster.VoiceAhh, roster.VoiceOpera} {
		if got := CategoryOf(id); got != CatVoice {
			t.Fatalf("CategoryOf(%q) = %q, want %q", id, got, CatVoice)
		}
	}
	// choir-ahh/choir-ooh/voice-bass/voice-pad were ear-tested 2026-07-08 and
	// ruled synths, not voices: renamed to ensemble-lead/ensemble-lead-dark/
	// ghost-bass/viola-pad and re-categorized out of CatVoice.
	synthCases := map[instrument.ID]Category{
		roster.EnsembleLead:     CatLead,
		roster.EnsembleLeadDark: CatLead,
		roster.GhostBass:        CatBass,
		roster.ViolaPad:         CatStrings,
	}
	for id, want := range synthCases {
		if got := CategoryOf(id); got != want {
			t.Errorf("CategoryOf(%q) = %q, want %q", id, got, want)
		}
	}
	// And plain bass ids must still classify as bass.
	if got := CategoryOf(roster.BassAcid); got != CatBass {
		t.Fatalf("bass-acid = %q, want %q", got, CatBass)
	}
}

// TestCategoryHasOneSpellingAndALabel: every category has a stable spelling
// (what code and logs say) and a display label (what the menu shows), and the
// zero value is no category at all.
func TestCategoryHasOneSpellingAndALabel(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Categories() {
		if c == CatNone || c.String() == "" || c.Label() == "" || seen[c.String()] {
			t.Errorf("category %d: spelling %q label %q", int(c), c.String(), c.Label())
		}
		seen[c.String()] = true
	}
	if CatNone.String() != "" || len(Categories()) != int(CatOther) {
		t.Fatalf("Categories() = %v", Categories())
	}
}
