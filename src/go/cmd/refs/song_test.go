//go:build !js

package main

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// Song-tier references pair a template with a recording of the SAME piece. A reference must name a real template, be a
// legitimate pairing and never be committed. Never close a gap with an NC/ND/SA or wrong-piece source: it reports a
// confident "wrong key" against a correct render, worse than no reference.

// A dangling reference is a catalogue defect.
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
