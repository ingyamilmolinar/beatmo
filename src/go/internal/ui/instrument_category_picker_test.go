//go:build test

package ui

import "testing"

func TestPickerCategoriesFromCanonicalTaxonomy(t *testing.T) {
	dv := newTestDrumView(t, 800, 600)
	dv.instRefreshDirty = true
	dv.refreshInstruments()
	if got := dv.instCatByID["kick"]; got != "Kick" {
		t.Fatalf("kick category = %q, want %q", got, "Kick")
	}
	for _, c := range dv.instCategories {
		if c == "Registered" {
			t.Fatalf("stale fallback category 'Registered' still present: %v", dv.instCategories)
		}
	}
}
