package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestComputeInstLabelHonorsOverride(t *testing.T) {
	dv := &DrumView{}
	audio.ClearInstrumentDisplayName(roster.Kick)
	if got := dv.computeInstLabel(roster.Kick); got != "Kick" {
		t.Fatalf("default: got %q want %q", got, "Kick")
	}
	audio.SetInstrumentDisplayName(roster.Kick, "Thumper")
	defer audio.ClearInstrumentDisplayName(roster.Kick)
	if got := dv.computeInstLabel(roster.Kick); got != "Thumper" {
		t.Fatalf("override: got %q want %q", got, "Thumper")
	}
}
