//go:build !test && !js

package audio

import "testing"

// TestSnarePostOrderTrap proves the base-snare POST op order (drive BEFORE tone,
// post_order=2) is observably distinct from the shared apply_post_params order
// (tone BEFORE drive, post_order=0) at the EXACT divergent point the |combo
// oracle fixture cannot reach — so post_order=2 is a load-bearing choice, not an
// arbitrary one.
//
// The base snare's bespoke inline post (the deleted render_snare_p) applied
// drive, THEN tone; the shared apply_post_params applies tone, THEN drive. The
// two orders only diverge when BOTH drive>0 AND the tone one-pole LP is active.
// The legacy snare tone branch is LP-ONLY for tone<0 (positive tone is a no-op).
// The oracle |combo case sets tone = -1 + 0.7*2 = +0.4 (positive ⇒ tone inert),
// so it can NOT tell the orders apart — both pass the combo fixture. This test
// sets tone=-0.5 (LP active) AND drive=0.5 simultaneously and asserts the two
// orders produce DIFFERENT bytes — i.e. the trap is real and the order matters.
//
// Correctness of the chosen order (post_order=2 == the legacy drive-before-tone
// sequence) is pinned separately and immutably by the drum-snare |tonedrive
// oracle fixture (TestSnareModularMatchesOracle / TestLegacyOracleGolden), whose
// hash was captured byte-for-byte from the deleted legacy render_snare_p at
// tone=-0.5,drive=0.5 — the exact divergent point. The |combo case CANNOT pin
// the order: it sets tone=+0.4 (positive ⇒ the legacy tone LP is inert), so both
// orders pass combo. |tonedrive is the load-bearing fixture for this trap.
func TestSnarePostOrderTrap(t *testing.T) {
	const sr = oracleSR
	const n = oracleSamples
	overlay := RecipeParams{"tone": -0.5, "drive": 0.5}
	merged := MergeRecipeDefaults("drum-snare", overlay)

	spec := snareVariantSpecs["drum-snare"]
	wired := snareWiredFields("drum-snare")

	// Production binding render (post_order=2, the legacy drive-before-tone order).
	mp2 := snareRecipeToModular("drum-snare", merged, spec.variant, spec.def, wired)
	if mp2.PostOrder != 2 {
		t.Fatalf("base-snare binding PostOrder = %v, want 2 (drive before tone)", mp2.PostOrder)
	}
	buf2 := make([]float32, n)
	renderModularP(buf2, sr, n, mp2)

	// Same params but the WRONG (shared) order: must DIVERGE, proving the trap is
	// observable at tone<0+drive (the orders are genuinely different here).
	mp0 := mp2
	mp0.PostOrder = 0
	buf0 := make([]float32, n)
	renderModularP(buf0, sr, n, mp0)

	if hashFloat32(buf2) == hashFloat32(buf0) {
		t.Fatalf("post_order=2 and post_order=0 produced identical bytes at tone<0+drive — the snare post-order trap should be observable here (drive-before-tone must differ from tone-before-drive)")
	}
}
