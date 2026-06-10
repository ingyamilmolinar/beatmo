package audio

import "testing"

// TestSeedInstrumentDefaultsToPlatform — the bootstrap defaults push must
// forward the shipped recipe defaults of every instrument bound to a SEEDED
// recipe (whose defaults diverge from the engine's plain render defaults),
// and must NOT push instruments whose defaults already match the plain
// render (base modular, drums, FM). This is what makes a divergent preset
// (modular-pad) render its intended sound in the browser before any edit,
// without duplicating defaults in JS.
func TestSeedInstrumentDefaultsToPlatform(t *testing.T) {
	pushed := map[string]RecipeParams{}
	restore := SwapPlatformInstrumentDefaultsPushForTest(func(id string, p RecipeParams) {
		pushed[id] = p
	})
	t.Cleanup(func() { SwapPlatformInstrumentDefaultsPushForTest(restore) })

	SeedInstrumentDefaultsToPlatform()

	// modular-pad diverges → must be pushed with its pad defaults.
	padDefaults, ok := pushed["modular-pad"]
	if !ok {
		t.Fatalf("modular-pad defaults were not pushed to the platform")
	}
	if got := padDefaults["filter_cutoff"]; got != 1200 {
		t.Errorf("modular-pad pushed filter_cutoff = %v, want 1200 (the pad seed)", got)
	}
	if got := padDefaults["amp_release"]; got != 1.4 {
		t.Errorf("modular-pad pushed amp_release = %v, want 1.4 (the pad seed)", got)
	}

	// Base modular has no seed (defaults == C render defaults) → no push needed.
	if _, ok := pushed["modular"]; ok {
		t.Errorf("base modular was pushed but its defaults match the plain render; no push needed")
	}
	// Modular-migrated instruments (snare → source==7, hihat → source==9, fm-bass →
	// source==10) MUST be pushed — their JS RENDER_INFO is paramBlock:'modular', so
	// an unedited migrated instrument renders through render_modular_p and needs the
	// binding-translated modular default block seeded (the all-identity modular
	// block would silence it, or — for FM — render the wrong preset). The native
	// hook receives the LEGACY-named recipe defaults; the modular-name translation
	// happens in the wasm push hook (modularPushParamsForInstrument), so we assert
	// only that the push occurred.
	//
	// EVERY legacy family has now migrated (FM in Phase-7, the LAST), so there is no
	// longer any "still-legacy must NOT be pushed" example — fm-bass flipped from
	// that role to a migrated-must-be-pushed instrument.
	if _, ok := pushed["snare"]; !ok {
		t.Fatalf("migrated drum instrument snare was NOT pushed; modular-migrated families need the bootstrap push")
	}
	if _, ok := pushed["hihat"]; !ok {
		t.Fatalf("migrated cymbal instrument hihat was NOT pushed; modular-migrated families need the bootstrap push")
	}
	if _, ok := pushed["fm-bass"]; !ok {
		t.Fatalf("migrated FM instrument fm-bass was NOT pushed; the FM family migrated to the modular engine (Phase-7) and needs the bootstrap push")
	}
}
