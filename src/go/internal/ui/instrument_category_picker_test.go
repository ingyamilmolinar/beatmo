//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestPickerCategoriesFromCanonicalTaxonomy(t *testing.T) {
	dv := newTestDrumView(t, 800, 600)
	dv.instRefreshDirty = true
	dv.refreshInstruments()
	if got := dv.instCatByID[roster.Kick]; got != audio.CatKick {
		t.Fatalf("kick category = %v, want %v", got, audio.CatKick)
	}
}
