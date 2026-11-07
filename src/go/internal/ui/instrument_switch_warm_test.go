package ui

import (
	"reflect"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Warming on the switch spares the first notes a cold per-pitch render during playback.
func TestSetInstrumentWarmsVoiceCache(t *testing.T) {
	g := New(testLogger)
	dv := g.drum
	dv.selRow = 0
	dv.Rows[0].Instrument = roster.Kick

	var gotID string
	var gotPitches []int
	called := 0
	orig := audio.WarmInstrument
	audio.WarmInstrument = func(id instrument.ID, pitches []int) {
		called++
		gotID = string(id)
		gotPitches = append([]int(nil), pitches...)
	}
	defer func() { audio.WarmInstrument = orig }()

	dv.SetInstrument(roster.Cello)

	if called != 1 {
		t.Fatalf("WarmInstrument called %d times, want exactly 1", called)
	}
	if gotID != "cello" {
		t.Fatalf("warm id = %q, want %q", gotID, "cello")
	}
	if !reflect.DeepEqual(gotPitches, audio.DefaultWarmPitches) {
		t.Fatalf("warm pitches = %v, want %v", gotPitches, audio.DefaultWarmPitches)
	}
}

func TestSetInstrumentSameIDNoWarm(t *testing.T) {
	g := New(testLogger)
	dv := g.drum
	dv.selRow = 0
	dv.Rows[0].Instrument = roster.Cello

	called := 0
	orig := audio.WarmInstrument
	audio.WarmInstrument = func(id instrument.ID, pitches []int) { called++ }
	defer func() { audio.WarmInstrument = orig }()

	dv.SetInstrument(roster.Cello)

	if called != 0 {
		t.Fatalf("WarmInstrument called %d times on same-id set, want 0", called)
	}
}
