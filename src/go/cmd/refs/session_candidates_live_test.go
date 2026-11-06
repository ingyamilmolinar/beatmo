//go:build !test && !js

package main

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
)

// Every candidate must (a) only carry overrides that land on the base's document
// (applied together, so a layer the candidate adds counts) and (b)
// render differently from the base — a candidate cannot silently be the
// current version, and a dead override key cannot ship (118 such keys are
// already on the dead-seed ratchet).
func TestShippedCandidates_AreLive(t *testing.T) {
	p, _ := defaultCandidatesPath()
	fams, err := loadCandidates(p)
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range fams {
		base, err := synthmatch.RenderInstrument(f.Base, 0, 44100, 1.0)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range f.Candidates {
			doc, ok := audio.EnsureInstrumentDoc(f.Base)
			if !ok {
				t.Fatalf("%s: base %q has no document", name, f.Base)
			}
			_, lost, err := audio.ApplyInstrumentFields(doc, c.Overrides)
			if err != nil {
				t.Errorf("%s/%s: %v", name, c.ID, err)
			}
			for _, k := range lost {
				t.Errorf("%s/%s: override %q never lands on %s (a dead key)", name, c.ID, k, f.Base)
			}
			w, err := synthmatch.RenderWithParams(f.Base, 0, 44100, 1.0, c.Overrides)
			if err != nil {
				t.Fatal(err)
			}
			diff := 0.0
			for i := range w.Samples {
				diff += math.Abs(w.Samples[i] - base.Samples[i])
			}
			if diff < 1e-6 {
				t.Errorf("%s/%s renders byte-identical to the base — overrides are dead", name, c.ID)
			}
		}
	}
}
