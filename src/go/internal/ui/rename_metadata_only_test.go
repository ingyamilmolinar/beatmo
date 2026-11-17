//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestRenameIsMetadataOnly(t *testing.T) {
	dv := newTestDrumViewForName(t)
	row := 0
	origID := dv.Rows[row].Instrument
	if origID == "" {
		t.Fatalf("row 0 has no instrument")
	}
	t.Cleanup(func() { audio.ClearInstrumentDisplayName(origID) })

	// Seed per-row audio state that the spec requires a rename to preserve
	// (rename is metadata-only — it must touch ONLY the display name).
	if len(rowEQ(dv.Rows[row]).GainsDB) == 0 {
		rowEQ(dv.Rows[row]).GainsDB = make([]float64, 10)
	}
	rowEQ(dv.Rows[row]).GainsDB[3] = 4.5

	dv.renameInstrumentTo(row, "My Custom Name")

	if dv.Rows[row].Instrument != origID {
		t.Fatalf("rename changed ID: got %q want %q (must be metadata-only)", dv.Rows[row].Instrument, origID)
	}
	// EQ must survive the rename verbatim.
	if rowEQ(dv.Rows[row]).GainsDB[3] != 4.5 {
		t.Fatalf("rename disturbed EQ: band3=%v want 4.5", rowEQ(dv.Rows[row]).GainsDB[3])
	}
	if dv.instDisplayLabel(origID) != "My Custom Name" {
		t.Fatalf("display label not updated: got %q", dv.instDisplayLabel(origID))
	}
	if audio.InstrumentDisplayName(origID) != "My Custom Name" {
		t.Fatalf("store not updated: got %q", audio.InstrumentDisplayName(origID))
	}
}
