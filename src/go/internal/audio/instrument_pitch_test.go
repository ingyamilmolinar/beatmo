package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestInstrumentPitchInfo_KnobsFoldIn(t *testing.T) {
	// Every pitch knob at 0 and an id the table does not know, so the offset is the declared computation alone.
	const inst = "test-pitch-inst-b"
	if err := InstallInstrumentDocForTest(inst, roster.Modular); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { RemoveInstrumentDocForTest(inst) })

	cases := []struct {
		name  string
		param string
		value float64
		want  float64
	}{
		{"octave up one", "osc_octave", 1, 12},
		{"octave down one", "osc_octave", -1, -12},
		{"detune +50 cents", "osc_detune", 50, 0.5},
		{"voice_freq_hz 440 = +12", "voice_freq_hz", 440, 12},
		{"voice_freq_hz 110 = -12", "voice_freq_hz", 110, -12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ResetInstrumentParams(inst)
			SetInstrumentParam(inst, tc.param, tc.value)
			off, _ := InstrumentPitchInfo(inst)
			if math.Abs(off-tc.want) > 1e-9 {
				t.Errorf("offset = %v, want %v", off, tc.want)
			}
		})
	}
}

func TestInstrumentPitchInfo_MelodicRecipeIsPitched(t *testing.T) {
	const inst = "test-pitch-inst-melodic"
	// Violin carries no pitch-shifting value, so it sits on the A3 baseline.
	_ = InstallInstrumentDocForTest(inst, roster.Violin)
	t.Cleanup(func() { RemoveInstrumentDocForTest(inst) })

	off, pitched := InstrumentPitchInfo(inst)
	if !pitched {
		t.Errorf("melodic recipe should be pitched")
	}
	if off != 0 {
		t.Errorf("untouched melodic offset = %v, want 0 (A3 baseline)", off)
	}
}

// The score importer compensates bass parts from this answer.
func TestInstrumentPitchInfo_MelodicRecipeSeedOffsetIsReported(t *testing.T) {
	const inst = "test-pitch-inst-melodic-shifted"
	_ = InstallInstrumentDocForTest(inst, roster.Cello)
	t.Cleanup(func() { RemoveInstrumentDocForTest(inst) })

	off, pitched := InstrumentPitchInfo(inst)
	if !pitched {
		t.Errorf("melodic recipe should be pitched")
	}
	if off != -12 {
		t.Errorf("cello offset = %v, want -12 (celloSeed osc_octave:-1)", off)
	}
}

// A head tuned to 55 Hz sounds A1: naming it is a measurement, not a guess.
func TestInstrumentPitchInfo_TunedDrumRecipeIsPitched(t *testing.T) {
	const inst = "test-pitch-inst-drum"
	_ = InstallInstrumentDocForTest(inst, roster.Kick) // declares fundamental=55 Hz
	t.Cleanup(func() { RemoveInstrumentDocForTest(inst) })

	off, pitched := InstrumentPitchInfo(inst)
	if !pitched {
		t.Fatalf("a drum recipe declaring a 55 Hz fundamental should be pitched")
	}
	if want := -24.0; math.Abs(off-want) > 0.01 {
		t.Errorf("drum-kick offset = %v, want %v (55 Hz is two octaves below A3)", off, want)
	}
}

// The limit that keeps "universal" from meaning "invented".
func TestInstrumentPitchInfo_NoiseRecipeNotPitched(t *testing.T) {
	const inst = "test-pitch-inst-noise"
	_ = InstallInstrumentDocForTest(inst, roster.Hihat) // declares no base frequency
	t.Cleanup(func() { RemoveInstrumentDocForTest(inst) })

	if _, pitched := InstrumentPitchInfo(inst); pitched {
		t.Errorf("a noise recipe with no declared fundamental must not be pitched")
	}
}

func TestInstrumentPitchInfo_SamplerTranspose(t *testing.T) {
	const inst = "test-pitch-inst-sampler"
	// No melodic recipe binding; a sample-edit transpose makes it pitched.
	SetSampleEdit(inst, SampleEdit{TransposeSemis: 5, DetuneCents: 50})
	t.Cleanup(func() { ClearSampleEdit(inst) })

	off, pitched := InstrumentPitchInfo(inst)
	if !pitched {
		t.Errorf("sampler with a transpose edit should be pitched")
	}
	if math.Abs(off-5.5) > 1e-9 {
		t.Errorf("offset = %v, want 5.5 (5 semis + 50 cents)", off)
	}
}

func TestInstrumentPitchInfo_UnknownInstrumentNotPitched(t *testing.T) {
	off, pitched := InstrumentPitchInfo("does-not-exist-xyz")
	if pitched {
		t.Errorf("unknown/unbound instrument should not be pitched")
	}
	if off != 0 {
		t.Errorf("unknown instrument offset = %v, want 0", off)
	}
}
