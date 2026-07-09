package audio

import (
	"slices"
	"testing"
)

func TestCategoryOfBuiltins(t *testing.T) {
	cases := map[string]CategoryID{
		"kick":                CatKick,
		"snare":               CatSnare,
		"hihat":               CatHiHat,
		"violin":              CatStrings,
		"guitar-electric":     CatGuitar,
		"organ":               CatKeys,
		"trumpet":             CatBrass,
		"flute":               CatWoodwind,
		"totally-unknown-xyz": CatOther,
	}
	for id, want := range cases {
		if got := CategoryOf(id); got != want {
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
	if !slices.Contains(got, "kick") {
		t.Fatalf("InstrumentsByCategory(CatKick) missing 'kick': %v", got)
	}
}

func TestVoiceCategoryClassification(t *testing.T) {
	for _, id := range []string{"voice-soprano", "voice-whisper", "voice-ahh", "voice-opera"} {
		if got := CategoryOf(id); got != CatVoice {
			t.Fatalf("CategoryOf(%q) = %q, want %q", id, got, CatVoice)
		}
	}
	// choir-ahh/choir-ooh/voice-bass/voice-pad were ear-tested 2026-07-08 and
	// ruled synths, not voices: renamed to ensemble-lead/ensemble-lead-dark/
	// ghost-bass/viola-pad and re-categorized out of CatVoice.
	synthCases := map[string]CategoryID{
		"ensemble-lead":      CatLead,
		"ensemble-lead-dark": CatLead,
		"ghost-bass":         CatBass,
		"viola-pad":          CatStrings,
	}
	for id, want := range synthCases {
		if got := CategoryOf(id); got != want {
			t.Errorf("CategoryOf(%q) = %q, want %q", id, got, want)
		}
	}
	// And plain bass ids must still classify as bass.
	if got := CategoryOf("bass-acid"); got != CatBass {
		t.Fatalf("bass-acid = %q, want %q", got, CatBass)
	}
}

func TestAssignCategoryOverride(t *testing.T) {
	AssignCategory("kick", CatPercussion)
	defer AssignCategory("kick", CatKick) // restore
	if got := CategoryOf("kick"); got != CatPercussion {
		t.Fatalf("AssignCategory override: got %q want %q", got, CatPercussion)
	}
}
