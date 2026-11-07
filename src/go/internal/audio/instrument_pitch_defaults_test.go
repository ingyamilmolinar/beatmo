package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A value the instrument SHIPS with (an osc_octave in its document) moves the sound as a user knob would, so the note
// must reflect it.
func TestInstrumentPitchInfoFoldsRecipeDefaults(t *testing.T) {
	cases := []struct {
		inst instrument.ID
		want float64 // semitones from A3
		why  string
	}{
		{roster.Cello, -12, "celloSeed carries osc_octave:-1"},
		{roster.CelloWarm, -12, "celloWarmSeed carries osc_octave:-1"},
		{roster.VoiceSoprano, +12, "voiceSopranoSeed carries osc_octave:+1"},
		{roster.BassGuitar, -12, "bassGuitarSeed carries osc_octave:-1 (bass register)"},
		{roster.OrganChurch, -12, "organChurchSeed carries osc_octave:-1"},
		{roster.PianoGrand, +0.015, "pianoGrandSeed carries osc_detune:1.5 cents"},
		{roster.DoubleBass, 0, "no octave offset — renders at nominal pitch"},
		{roster.Violin, 0, "no octave offset"},
		{roster.Flute, 0, "no octave offset"},
	}
	for _, tc := range cases {
		got, pitched := InstrumentPitchInfo(tc.inst)
		if !pitched {
			t.Errorf("%s: expected a pitched instrument", tc.inst)
			continue
		}
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("InstrumentPitchInfo(%q) = %+.4f semis, want %+.4f (%s)",
				tc.inst, got, tc.want, tc.why)
		}
	}
}

// Or re-tuning in the Synth tab would stop moving the displayed note.
func TestInstrumentPitchInfoUserOverrideBeatsDefault(t *testing.T) {
	const inst = roster.Cello // default osc_octave:-1
	if base, _ := InstrumentPitchInfo(inst); math.Abs(base-(-12)) > 1e-9 {
		t.Fatalf("precondition: cello default offset = %+.4f, want -12", base)
	}
	SetInstrumentParams(inst, RecipeParams{"osc_octave": 0})
	t.Cleanup(func() { SetInstrumentParams(inst, RecipeParams{"osc_octave": -1}) })

	got, _ := InstrumentPitchInfo(inst)
	if math.Abs(got) > 1e-9 {
		t.Errorf("after osc_octave=0 override, offset = %+.4f semis, want 0 "+
			"(the user override must beat the recipe default)", got)
	}
}
