//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

func TestUploadWAVMultipleAllowsInstrumentChange(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	audio.Register("foo", nil)
	g.drum.AddInstrument("foo")
	audio.Register("bar", nil)
	g.drum.AddInstrument("bar")

	if g.drum.Rows[0].Instrument != "bar" {
		t.Fatalf("expected instrument to be bar, got %s", g.drum.Rows[0].Instrument)
	}

	before := g.drum.Rows[0].Instrument
	g.drum.CycleInstrument()
	if g.drum.Rows[0].Instrument == before {
		t.Fatalf("instrument did not change after cycling")
	}
	audio.ResetInstruments()
}
