package ui

import (
	"reflect"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestSetInstrumentWarmsVoiceCache: switching a row to a new instrument must
// pre-warm that instrument's voice cache (audio.WarmInstrument) so the first
// notes after the switch don't pay a cold per-pitch render during playback.
func TestSetInstrumentWarmsVoiceCache(t *testing.T) {
	g := New(testLogger)
	dv := g.drum
	dv.selRow = 0
	dv.Rows[0].Instrument = "kick"

	var gotID string
	var gotPitches []int
	called := 0
	orig := audio.WarmInstrument
	audio.WarmInstrument = func(id string, pitches []int) {
		called++
		gotID = id
		gotPitches = append([]int(nil), pitches...)
	}
	defer func() { audio.WarmInstrument = orig }()

	dv.SetInstrument("cello")

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

// TestSetInstrumentSameIDNoWarm: re-selecting the SAME instrument must not
// trigger a warm (no change, nothing to render).
func TestSetInstrumentSameIDNoWarm(t *testing.T) {
	g := New(testLogger)
	dv := g.drum
	dv.selRow = 0
	dv.Rows[0].Instrument = "cello"

	called := 0
	orig := audio.WarmInstrument
	audio.WarmInstrument = func(id string, pitches []int) { called++ }
	defer func() { audio.WarmInstrument = orig }()

	dv.SetInstrument("cello")

	if called != 0 {
		t.Fatalf("WarmInstrument called %d times on same-id set, want 0", called)
	}
}
