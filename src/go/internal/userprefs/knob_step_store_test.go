package userprefs

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestKnobStepStoreRoundTrips verifies marshalPrefs + parsePrefs preserve
// the full step map across a serialize/parse cycle.
func TestKnobStepStoreRoundTrips(t *testing.T) {
	steps := map[string]float64{"filter_cutoff": 100, "amp_attack": 0.01}
	b, err := marshalPrefs(prefsSnapshot{knobSteps: steps})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	snap, err := parsePrefs(b, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if snap.knobSteps["filter_cutoff"] != 100 {
		t.Fatalf("filter_cutoff: got %v, want 100", snap.knobSteps["filter_cutoff"])
	}
	if snap.knobSteps["amp_attack"] != 0.01 {
		t.Fatalf("amp_attack: got %v, want 0.01", snap.knobSteps["amp_attack"])
	}
}

// TestKnobStepStoreEmptyOmitted verifies an empty knob-steps map is omitted
// from the document entirely (omitempty), and parses back as an empty —
// never nil — map.
func TestKnobStepStoreEmptyOmitted(t *testing.T) {
	b, err := marshalPrefs(prefsSnapshot{favorites: map[instrument.ID]bool{roster.Kick: true}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := raw["knob_steps"]; present {
		t.Errorf("empty knob_steps section must be omitted, got %s", b)
	}
	snap, err := parsePrefs(b, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(snap.knobSteps) != 0 {
		t.Fatalf("expected no knob steps, got %+v", snap.knobSteps)
	}
	if snap.knobSteps == nil {
		t.Error("load path must return a non-nil map")
	}
}

// A document with no knob_steps key reads back as an empty map without error.
func TestKnobStepStoreAbsentSectionCompat(t *testing.T) {
	b, err := marshalPrefs(prefsSnapshot{
		favorites:  map[instrument.ID]bool{roster.Snare: true},
		audioPanel: AudioPanelStateDoc{SynthGraphDetails: true},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	snap, err := parsePrefs(b, nil)
	if err != nil {
		t.Fatalf("parsePrefs: %v", err)
	}
	if len(snap.knobSteps) != 0 {
		t.Fatalf("expected empty map, got %+v", snap.knobSteps)
	}
	// The rest of the document must survive intact.
	if !snap.favorites[roster.Snare] {
		t.Error("favorites lost")
	}
	if !snap.audioPanel.SynthGraphDetails {
		t.Errorf("audioPanel lost: %+v", snap.audioPanel)
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
	b, err := marshalPrefs(prefsSnapshot{knobSteps: steps})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	snap, err := parsePrefs(b, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, bad := range []string{"nan", "inf", "neginf"} {
		if _, ok := snap.knobSteps[bad]; ok {
			t.Errorf("%s entry must be dropped", bad)
		}
	}
	if snap.knobSteps["good"] != 100.0 {
		t.Errorf("finite entry must survive, got %v", snap.knobSteps["good"])
	}
}
