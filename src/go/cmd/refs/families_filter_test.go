//go:build !js

package main

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Re-auditing after a pipeline change is targeted: a computable set of families has stale verdicts, and the rest are
// still valid.

func famsFixture() []auditionFamily {
	return []auditionFamily{
		{Key: roster.Cello, Members: []instrument.ID{roster.Cello, roster.CelloWarm}},
		{Key: roster.Flute, Members: []instrument.ID{roster.Flute}},
		{Key: roster.Oboe, Members: []instrument.ID{roster.Oboe}},
		{Key: roster.Violin, Members: []instrument.ID{roster.Violin}},
	}
}

func TestFilterFamilies_EmptyKeepsEverything(t *testing.T) {
	got, err := filterFamilies(famsFixture(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Errorf("an empty -families must not filter, got %d of 4", len(got))
	}
}

func TestFilterFamilies_KeepsOnlyNamedAndPreservesOrder(t *testing.T) {
	got, err := filterFamilies(famsFixture(), "violin,cello")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d families, want 2", len(got))
	}
	// Catalogue order wins over the order they were typed in, so a session is
	// reproducible regardless of how the list was pasted together.
	if got[0].Key != roster.Cello || got[1].Key != roster.Violin {
		t.Errorf("got %s,%s want cello,violin (catalogue order)", got[0].Key, got[1].Key)
	}
}

func TestFilterFamilies_ToleratesWhitespaceAndEmptyEntries(t *testing.T) {
	got, err := filterFamilies(famsFixture(), " cello , ,flute,")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("got %d families, want 2", len(got))
	}
}

// A typo must fail loudly. Silently auditing nothing (or, worse, silently
// auditing everything) after a mistyped family name would be a long, confusing
// listening session spent on the wrong instruments.
func TestFilterFamilies_UnknownNameIsAnError(t *testing.T) {
	_, err := filterFamilies(famsFixture(), "cello,celo,violin")
	if err == nil {
		t.Fatal("a misspelled family name must be rejected")
	}
	if !strings.Contains(err.Error(), "celo") {
		t.Errorf("the error must name the offending entry, got %q", err)
	}
}
