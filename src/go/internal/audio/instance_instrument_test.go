package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestEnsureInstanceInstrument(t *testing.T) {
	withDefaultAudio(t) // load roster.All + ResetInstruments cleanup

	// Precondition: the base is registered with a document; the variant is not.
	if !idListContains(Instruments(), roster.Organ) {
		t.Fatalf("precondition: base 'organ' not registered")
	}
	if !HasInstrumentDoc(roster.Organ) {
		t.Fatalf("precondition: base 'organ' has no instrument document")
	}
	if idListContains(Instruments(), "organ--2") {
		t.Fatalf("precondition: variant 'organ-2' already registered")
	}

	EnsureInstanceInstrument("organ--2")

	// (a) The variant is now in the playable registry.
	if !idListContains(Instruments(), "organ--2") {
		t.Fatalf("organ-2 not in Instruments() after EnsureInstanceInstrument")
	}

	// (b) Its own document, a copy of the base's library entry.
	if d, ok := InstrumentDoc("organ--2"); !ok || d.Origin != roster.Organ {
		t.Fatalf("organ--2 document origin = %q (ok %v), want organ", d.Origin, ok)
	}

	// The voice is rendered from the flattened block and render settings, so both must match field for field.
	baseDoc, _ := InstrumentDoc(roster.Organ)
	varDoc, _ := InstrumentDoc("organ--2")
	if baseDoc.Render != varDoc.Render {
		t.Fatalf("organ-2 render settings %+v != organ's %+v", varDoc.Render, baseDoc.Render)
	}
	baseBlock, varBlock := FlatFieldMap(baseDoc), FlatFieldMap(varDoc)
	for name, b := range baseBlock {
		if v := varBlock[name]; v != b && !(math.IsNaN(b) && math.IsNaN(v)) {
			t.Fatalf("organ-2 field %s = %v, organ's = %v", name, v, b)
		}
	}
}

// TestEnsureInstanceInstrumentIdempotentAndGuards covers the early-return paths.
func TestEnsureInstanceInstrumentIdempotentAndGuards(t *testing.T) {
	withDefaultAudio(t)

	// No instance suffix → no-op (base==id).
	before := len(Instruments())
	EnsureInstanceInstrument(roster.Organ)
	if len(Instruments()) != before {
		t.Fatalf("EnsureInstanceInstrument(organ) mutated the registry")
	}

	// Unknown base → no-op (nothing to alias).
	EnsureInstanceInstrument("nonsense--2")
	if idListContains(Instruments(), "nonsense--2") {
		t.Fatalf("EnsureInstanceInstrument registered a variant of an unknown base")
	}

	// Idempotent: second call after a successful promotion does not duplicate.
	EnsureInstanceInstrument("organ--2")
	n := idListCount(Instruments(), "organ--2")
	EnsureInstanceInstrument("organ--2")
	if got := idListCount(Instruments(), "organ--2"); got != n || got != 1 {
		t.Fatalf("organ-2 registered %d times (was %d), want exactly 1", got, n)
	}
}

func idListContains(ids []instrument.ID, id instrument.ID) bool {
	for _, s := range ids {
		if s == id {
			return true
		}
	}
	return false
}

func idListCount(ids []instrument.ID, id instrument.ID) int {
	n := 0
	for _, s := range ids {
		if s == id {
			n++
		}
	}
	return n
}

// An instance id is not a roster id, so no static loudness alias can cover it: without seeding, violin--2 would play
// at DefaultAmplitude instead of violin's measured level.
func TestEnsureInstanceInstrumentInheritsBaseLoudness(t *testing.T) {
	withDefaultAudio(t) // load roster.All + ResetInstruments cleanup

	base := ConfigForInstrument(roster.Violin).Amplitude
	if base == DefaultAmplitude {
		t.Fatalf("precondition: 'violin' has no measured loudness distinct from DefaultAmplitude (%v); test cannot detect the fallback bug", DefaultAmplitude)
	}

	EnsureInstanceInstrument("violin--2")

	if got := ConfigForInstrument("violin--2").Amplitude; got != base {
		t.Fatalf("ConfigForInstrument(violin-2).Amplitude = %v, want base violin's %v", got, base)
	}
}
