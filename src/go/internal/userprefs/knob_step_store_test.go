package userprefs

import (
	"math"
	"testing"
)

// TestKnobStepStoreRoundTrips verifies marshalPrefsV6 + parseKnobStepsFromPrefs
// preserve the full step map across a marshal/unmarshal cycle.
func TestKnobStepStoreRoundTrips(t *testing.T) {
	steps := map[string]float64{"filter_cutoff": 100, "amp_attack": 0.01}
	b, err := marshalPrefsV6(nil, nil, nil, AudioPanelStateDoc{}, nil, nil, steps)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := parseKnobStepsFromPrefs(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got["filter_cutoff"] != 100 {
		t.Fatalf("filter_cutoff: got %v, want 100", got["filter_cutoff"])
	}
	if got["amp_attack"] != 0.01 {
		t.Fatalf("amp_attack: got %v, want 0.01", got["amp_attack"])
	}
}

// TestKnobStepStoreEmptyOmitted verifies that an absent knob-steps section in a
// V5-era prefs file is correctly parsed as an empty map (backward compat).
func TestKnobStepStoreEmptyOmitted(t *testing.T) {
	// No knob steps => V6 serialises as V5 (section omitted). Parse must
	// succeed and return an empty map, never an error.
	b, err := marshalPrefsV6(map[string]bool{"kick": true}, nil, nil, AudioPanelStateDoc{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := parseKnobStepsFromPrefs(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no knob steps, got %+v", got)
	}
}

// TestKnobStepStoreV5BackwardCompat verifies that a V5 prefs doc (no
// knob_steps key) is read back as an empty map without error.
func TestKnobStepStoreV5BackwardCompat(t *testing.T) {
	// Simulate an old V5 prefs file by marshalling without a knob-steps map.
	b, err := marshalPrefsV5(
		map[string]bool{"snare": true},
		nil, nil,
		AudioPanelStateDoc{SpectrumSlopeIdx: 1},
		nil, nil,
	)
	if err != nil {
		t.Fatalf("marshalPrefsV5: %v", err)
	}
	got, err := parseKnobStepsFromPrefs(b)
	if err != nil {
		t.Fatalf("parseKnobStepsFromPrefs on V5 data: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map for V5 data, got %+v", got)
	}
}

// TestKnobStepStoreDropsNonFinite verifies that NaN / Inf entries are
// rejected at the marshal boundary (same rule as RecipeOverrides).
func TestKnobStepStoreDropsNonFinite(t *testing.T) {
	steps := map[string]float64{
		"good":   100.0,
		"nan":    math.NaN(),
		"inf":    math.Inf(1),
		"neginf": math.Inf(-1),
	}
	b, err := marshalPrefsV6(nil, nil, nil, AudioPanelStateDoc{}, nil, nil, steps)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := parseKnobStepsFromPrefs(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := got["nan"]; ok {
		t.Errorf("NaN entry must be dropped")
	}
	if _, ok := got["inf"]; ok {
		t.Errorf("+Inf entry must be dropped")
	}
	if _, ok := got["neginf"]; ok {
		t.Errorf("-Inf entry must be dropped")
	}
	if got["good"] != 100.0 {
		t.Errorf("finite entry must survive, got %v", got["good"])
	}
}
