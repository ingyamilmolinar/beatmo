package audio

import "testing"

func TestFactoryRecipeBaseIDFallback(t *testing.T) {
	base := factoryRecipeForInstrument("organ")
	if base == "" {
		t.Fatalf("precondition: 'organ' has no factory recipe")
	}
	if got := factoryRecipeForInstrument("organ-2"); got != base {
		t.Fatalf("organ-2 recipe = %q, want base %q", got, base)
	}
	// A genuinely unbound id with no base still yields empty.
	if got := factoryRecipeForInstrument("nonsense-xyz"); got != "" {
		t.Fatalf("nonsense-xyz recipe = %q, want empty", got)
	}
	// An explicitly-bound variant must win over the base-id fallback: "snare-1"
	// has its OWN binding and must NOT collapse to the base "snare" recipe.
	snare1 := factoryRecipeForInstrument("snare-1")
	snareBase := factoryRecipeForInstrument("snare")
	if snare1 == "" {
		t.Fatalf("precondition: 'snare-1' has no factory recipe")
	}
	if snare1 == snareBase {
		t.Fatalf("snare-1 recipe %q collapsed to base snare recipe %q; explicit binding must win", snare1, snareBase)
	}
}
