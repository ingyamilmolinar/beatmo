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

func TestAssignCategoryOverride(t *testing.T) {
	AssignCategory("kick", CatPercussion)
	defer AssignCategory("kick", CatKick) // restore
	if got := CategoryOf("kick"); got != CatPercussion {
		t.Fatalf("AssignCategory override: got %q want %q", got, CatPercussion)
	}
}
