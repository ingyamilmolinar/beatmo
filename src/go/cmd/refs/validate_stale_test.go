//go:build !js

package main

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestStaleness(t *testing.T) {
	if s := staleness(earVerdict{Family: roster.Oboe, Options: []instrument.ID{roster.Oboe}}); s != "unhashed" {
		t.Fatalf("no hash → unhashed, got %q", s)
	}
	if s := staleness(earVerdict{Family: roster.Oboe, Options: []instrument.ID{roster.Oboe}, Chosen: roster.Oboe, RecipeSHA: "deadbeefdeadbeef"}); s != "STALE" {
		t.Fatalf("mismatching hash → STALE, got %q", s)
	}
}
