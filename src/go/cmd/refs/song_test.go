//go:build !js

package main

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// Song-tier references pair a template with a recording of the
// SAME piece, so `refs` can compare a rendered circuit against a real
// performance.
//
// The catalogue currently holds NONE. The seven that existed were all keyed to
// templates removed by the public-domain curation pass (bach-toccata,
// bach-prelude-c, bach-cello-prelude, mozart-k545, vivaldi-spring,
// pachelbel-violin, marcello-oboe); with their templates gone the references
// pointed at nothing, so they were pruned rather than left dangling.
//
// Re-seeding them for the current repertoire is real work and deliberately not
// bundled into that pass: each entry needs a recording of the exact movement,
// downloaded and hashed. The Hauptstimme corpus ships
// `_alignment.csv` files referencing public-domain IMSLP recordings of the same
// movements, which is the obvious starting point.
//
// The invariants below therefore no longer assert "every public-domain template
// HAS a reference" — with 24 public-domain templates and no references that
// would be a standing failure describing a known gap. What they assert instead
// is integrity: a reference that exists must name a real template, must be a
// legitimate pairing, and must never be committed. `refs doctor` reports the
// coverage gap.
//
// Do NOT close the gap by adding an NC/ND/SA source, or a same-composer-
// wrong-piece source: song-tier comparison checks tempo, rhythm, key and mix,
// so a wrong-piece reference reports a confident "wrong key" against a template
// that is rendering its score correctly — a false negative, worse than no
// reference at all. An honest gap is correct.

// TestEverySongReferenceNamesARealTemplate is the integrity direction of the
// old coverage test: a dangling reference is a catalogue defect, and it is
// exactly what the curation pass created before the prune.
func TestEverySongReferenceNamesARealTemplate(t *testing.T) {
	c, err := wavref.Catalogue()
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	stems, err := templateStems()
	if err != nil {
		t.Fatalf("templateStems: %v", err)
	}
	known := map[string]bool{}
	for _, s := range stems {
		known[s] = true
	}
	for _, r := range c.Refs {
		if r.Tier != wavref.TierSong {
			continue
		}
		if r.Template == "" {
			t.Errorf("song ref %q names no template", r.ID)
			continue
		}
		if !known[r.Template] {
			t.Errorf("song ref %q points at template %q, which does not exist — "+
				"remove the reference or restore the template", r.ID, r.Template)
		}
	}
}
