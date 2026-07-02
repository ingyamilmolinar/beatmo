package audio

import (
	"math"
	"testing"
)

// TestEnsureInstanceInstrument verifies that promoting an instance variant
// ("organ-2") yields a registered, available, recipe-bound, render-equivalent
// instrument identical to its base ("organ").
func TestEnsureInstanceInstrument(t *testing.T) {
	withDefaultAudio(t) // load BuiltinInstrumentIDs + ResetInstruments cleanup

	// Precondition: the base is registered and recipe-bound; the variant is not.
	if !idListContains(Instruments(), "organ") {
		t.Fatalf("precondition: base 'organ' not registered")
	}
	if RecipeForInstrument("organ") == "" {
		t.Fatalf("precondition: base 'organ' has no recipe binding")
	}
	if idListContains(Instruments(), "organ-2") {
		t.Fatalf("precondition: variant 'organ-2' already registered")
	}

	EnsureInstanceInstrument("organ-2")

	// (a) The variant is now in the playable registry.
	if !idListContains(Instruments(), "organ-2") {
		t.Fatalf("organ-2 not in Instruments() after EnsureInstanceInstrument")
	}

	// (b) Same recipe as the base — drives the recipe-vs-legacy render decision.
	if got, want := RecipeForInstrument("organ-2"), RecipeForInstrument("organ"); got != want {
		t.Fatalf("RecipeForInstrument(organ-2)=%q, want base %q", got, want)
	}

	// (c) Render equivalence — the crux. organ-2 must produce an IDENTICAL
	// waveform to organ. RenderInstrumentPreview is a pure function of (recipe,
	// per-id params), both identical for a freshly-promoted variant, so any
	// divergence (e.g. the variant collapsing to a generic tone) fails here.
	// This directly guards the failure mode instance-promotion exists to prevent.
	// NB: compare NaN-tolerantly — the organ recipe's preview legitimately emits
	// NaN in its tail (a pre-existing preview quirk affecting the base too), and
	// NaN != NaN would make reflect.DeepEqual reject two identical waveforms.
	baseWave := RenderInstrumentPreview("organ", 100)
	varWave := RenderInstrumentPreview("organ-2", 100)
	if len(varWave) == 0 {
		t.Fatalf("organ-2 produced no preview samples")
	}
	if len(varWave) != len(baseWave) {
		t.Fatalf("organ-2 preview len %d != base len %d", len(varWave), len(baseWave))
	}
	for i := range baseWave {
		b, v := baseWave[i], varWave[i]
		if b != v && !(math.IsNaN(b) && math.IsNaN(v)) {
			t.Fatalf("organ-2 preview diverges from organ at sample %d: base=%v variant=%v", i, b, v)
		}
	}
}

// TestEnsureInstanceInstrumentIdempotentAndGuards covers the early-return paths.
func TestEnsureInstanceInstrumentIdempotentAndGuards(t *testing.T) {
	withDefaultAudio(t)

	// No instance suffix → no-op (base==id).
	before := len(Instruments())
	EnsureInstanceInstrument("organ")
	if len(Instruments()) != before {
		t.Fatalf("EnsureInstanceInstrument(organ) mutated the registry")
	}

	// Unknown base → no-op (nothing to alias).
	EnsureInstanceInstrument("nonsense-2")
	if idListContains(Instruments(), "nonsense-2") {
		t.Fatalf("EnsureInstanceInstrument registered a variant of an unknown base")
	}

	// Idempotent: second call after a successful promotion does not duplicate.
	EnsureInstanceInstrument("organ-2")
	n := idListCount(Instruments(), "organ-2")
	EnsureInstanceInstrument("organ-2")
	if got := idListCount(Instruments(), "organ-2"); got != n || got != 1 {
		t.Fatalf("organ-2 registered %d times (was %d), want exactly 1", got, n)
	}
}

func idListContains(ids []string, id string) bool {
	for _, s := range ids {
		if s == id {
			return true
		}
	}
	return false
}

func idListCount(ids []string, id string) int {
	n := 0
	for _, s := range ids {
		if s == id {
			n++
		}
	}
	return n
}
