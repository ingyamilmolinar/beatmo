package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The render path flattens the instrument's document (then sets the pitch
// field), so a pitch-shifting value the instrument SHIPS with — an osc_octave
// in its document, say — moves the sounding frequency exactly as a user knob
// would.
//
// InstrumentPitchInfo used to read only the user-override map, so those seed
// offsets were invisible to it: it reported +0 semitones for instruments the
// engine renders a full octave away. That broke its documented contract
// ("the displayed note is exactly what the engine sounds") and, because it is
// the only programmatic answer to "does this instrument shift pitch?", it left
// the score importer hand-maintaining the same knowledge in a comment — where
// it drifted (double-bass was compensated as if it had an octave offset it
// does not have).
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

// A user edit must still WIN over the shipped value (it replaces it in the
// document) — otherwise re-tuning an instrument in the
// Synth tab would stop moving its displayed note.
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
