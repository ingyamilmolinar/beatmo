package audio

import (
	"math"
	"testing"
)

// pitchTestRecipe registers a recipe carrying the pitch-affecting knobs so the
// offset math can be exercised, returning a cleanup func. Category is
// irrelevant to the pitched decision (that keys off melodicRecipeIDs), so these
// synthetic recipes are used only for the offset-math cases.
func pitchTestRecipe(t *testing.T, id string) func() {
	t.Helper()
	reg := RecipeRegistration{
		ID:          id,
		DisplayName: id,
		Category:    "modular",
		Params: []ParamDef{
			{Name: "osc_octave", Min: -2, Max: 2, Default: 0},
			{Name: "osc_detune", Min: -100, Max: 100, Default: 0},
			{Name: "fundamental", Min: 0, Max: 4000, Default: 0},
		},
		New: func() SynthRecipe { return newFakeRecipe(id).New() },
	}
	RegisterRecipe(reg)
	return func() { unregisterRecipeForTest(id) }
}

func TestInstrumentPitchInfo_KnobsFoldIn(t *testing.T) {
	const rec, inst = "test-pitch-knobs", "test-pitch-inst-b"
	t.Cleanup(pitchTestRecipe(t, rec))
	BindInstrumentToRecipe(inst, rec)
	t.Cleanup(func() {
		ResetInstrumentParams(inst)
		BindInstrumentToRecipe(inst, "")
	})

	cases := []struct {
		name  string
		param string
		value float64
		want  float64
	}{
		{"octave up one", "osc_octave", 1, 12},
		{"octave down one", "osc_octave", -1, -12},
		{"detune +50 cents", "osc_detune", 50, 0.5},
		{"fundamental 440 = +12", "fundamental", 440, 12},
		{"fundamental 110 = -12", "fundamental", 110, -12},
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
	// synth-modular-cello is in melodicRecipeIDs (a real bowed-string recipe).
	BindInstrumentToRecipe(inst, "synth-modular-cello")
	t.Cleanup(func() { BindInstrumentToRecipe(inst, "") })

	off, pitched := InstrumentPitchInfo(inst)
	if !pitched {
		t.Errorf("melodic recipe should be pitched")
	}
	if off != 0 {
		t.Errorf("untouched melodic offset = %v, want 0 (A3 baseline)", off)
	}
}

func TestInstrumentPitchInfo_DrumRecipeNotPitched(t *testing.T) {
	const inst = "test-pitch-inst-drum"
	BindInstrumentToRecipe(inst, "drum-kick") // not in melodicRecipeIDs
	t.Cleanup(func() { BindInstrumentToRecipe(inst, "") })

	if _, pitched := InstrumentPitchInfo(inst); pitched {
		t.Errorf("drum recipe must not be pitched")
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
