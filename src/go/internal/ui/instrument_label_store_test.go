package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestComputeInstLabelHonorsOverride(t *testing.T) {
	dv := &DrumView{}
	audio.ClearInstrumentDisplayName("kick")
	if got := dv.computeInstLabel("kick"); got != "Kick" {
		t.Fatalf("default: got %q want %q", got, "Kick")
	}
	audio.SetInstrumentDisplayName("kick", "Thumper")
	defer audio.ClearInstrumentDisplayName("kick")
	if got := dv.computeInstLabel("kick"); got != "Thumper" {
		t.Fatalf("override: got %q want %q", got, "Thumper")
	}
}
