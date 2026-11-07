package synthdiag

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

func TestStages_GenSlotsAblateBySourceNotGain(t *testing.T) {
	for _, s := range Stages() {
		if len(s.Name) >= 3 && s.Name[:3] == "gen" {
			if _, ok := s.Overrides[s.Name+"_source"]; !ok {
				t.Errorf("%s must ablate via %s_source=0 (gain 0 means 'unconfigured' for transplanted sources)", s.Name, s.Name)
			}
			if _, bad := s.Overrides[s.Name+"_gain"]; bad {
				t.Errorf("%s must not touch _gain", s.Name)
			}
		}
	}
}

func TestActiveStages_OnlyActiveOnes(t *testing.T) {
	merged := map[string]float64{"osc_enabled": 1, "gen1_source": 1, "gen2_source": 0, "unison_voices": 1, "body_mix": 0, "lfo_enabled": 1}
	names := map[string]bool{}
	for _, s := range ActiveStages(merged) {
		names[s.Name] = true
	}
	for _, want := range []string{"osc", "gen1", "lfo"} {
		if !names[want] {
			t.Errorf("%s should be active", want)
		}
	}
	for _, no := range []string{"gen2", "ensemble", "resonator"} {
		if names[no] {
			t.Errorf("%s should be inactive", no)
		}
	}
}

// An ignored override renders identically and reports zero leverage: the false "does not matter" this tool exists to
// prevent.
func TestStages_EveryOverrideKeyIsARealParam(t *testing.T) {
	known := map[string]bool{}
	for _, d := range audio.ModularSynthParamDefs() {
		known[d.Name] = true
	}
	for _, s := range Stages() {
		for k := range s.Overrides {
			if !known[k] {
				t.Errorf("stage %s: override key %q is not in audio.ModularSynthParamDefs()", s.Name, k)
			}
		}
	}
}

func TestTopStages_RanksDropsNaNAndBreaksTiesStably(t *testing.T) {
	rep := Report{
		Stages: []string{"osc", "gen1", "gen2", "filter"},
		Axes:   []string{"a.x", "a.y"},
		Names:  []string{"x", "y"},
		Sens: [][]float64{
			{0.5, 1},
			{2.0, 1},
			{0.5, 1},
			{math.NaN(), 1},
		},
	}
	got := rep.TopStages("a.x", 3)
	want := []StageSens{{"gen1", 2.0}, {"osc", 0.5}, {"gen2", 0.5}}
	if len(got) != len(want) {
		t.Fatalf("TopStages = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TopStages = %+v, want %+v (ties must keep Stages() order; NaN must be dropped)", got, want)
		}
	}
	if rep.TopStages("a.nope", 3) != nil {
		t.Error("an unknown axis must return nil, not an empty ranking")
	}
}

func TestStageSensitivity_SubtractsThePerKindNoEffectFloor(t *testing.T) {
	secs := fingerprint.Descriptor{
		ID: "t.seconds", Kind: fingerprint.KindSeconds,
		Applies: fingerprint.AlwaysApplies,
		Extract: func(*fingerprint.Bundle) fingerprint.Measurement { return fingerprint.Scalar(1) },
	}
	frac := fingerprint.Descriptor{
		ID: "t.fraction", Kind: fingerprint.KindFraction,
		Applies: fingerprint.AlwaysApplies,
		Extract: func(*fingerprint.Bundle) fingerprint.Measurement { return fingerprint.Scalar(1) },
	}

	full := fingerprint.Scalar(1.0)

	if floor := noEffectFloor(secs, full); floor <= 0 {
		t.Fatalf("a KindSeconds descriptor compared with itself must have a NON-zero Norm floor (got %v); "+
			"if this is ever 0 the subtraction below is pointless", floor)
	}
	if floor := noEffectFloor(frac, full); floor != 0 {
		t.Errorf("a KindFraction descriptor floors at 0, got %v", floor)
	}

	// A stage with no effect reads exactly 0 on BOTH kinds after subtraction.
	for _, d := range []fingerprint.Descriptor{secs, frac} {
		v, ok := stageSensitivity(d, full, full)
		if !ok {
			t.Fatalf("%s: identical measurements must be comparable", d.ID)
		}
		if v != 0 {
			t.Errorf("%s: a no-effect ablation must read 0, got %v", d.ID, v)
		}
	}

	// A real change still registers, and stays positive.
	v, ok := stageSensitivity(secs, full, fingerprint.Scalar(4.0))
	if !ok || v <= 0 {
		t.Errorf("a 4x change on a seconds axis must be positive, got %v (ok=%v)", v, ok)
	}

	// An unmeasurable side is reported as not-comparable, never as 0 leverage.
	if _, ok := stageSensitivity(secs, full, fingerprint.Unavailable("no event")); ok {
		t.Error("an unavailable measurement must report ok=false, not a sensitivity of 0")
	}
}
