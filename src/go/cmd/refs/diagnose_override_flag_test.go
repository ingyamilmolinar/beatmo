//go:build !js

package main

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Shapes that would otherwise render the SHIPPED sound while the operator believes it is a candidate: a malformed pair,
// a non-numeric value, a key given twice with different values.
func TestParseOverrideList(t *testing.T) {
	t.Run("empty yields nil", func(t *testing.T) {
		for _, in := range []string{"", "   ", ",", " , "} {
			got, err := parseOverrideList(in)
			if err != nil {
				t.Fatalf("parseOverrideList(%q) error: %v", in, err)
			}
			if got != nil {
				t.Errorf("parseOverrideList(%q) = %v, want nil (no overrides = the shipped sound)", in, got)
			}
		}
	})

	t.Run("parses pairs with surrounding space", func(t *testing.T) {
		got, err := parseOverrideList(" amp_attack=1.0 , gen1_env_tail_mix=0 ,lfo_enabled=0")
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]float64{"amp_attack": 1.0, "gen1_env_tail_mix": 0, "lfo_enabled": 0}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %v, want %v", k, got[k], v)
			}
		}
	})

	t.Run("rejects malformed input", func(t *testing.T) {
		for _, in := range []string{
			"amp_attack",          // no '='
			"=1.0",                // no key
			"amp_attack=fast",     // not a float
			"gain=0.5,gain=0.6",   // duplicate key: which one wins is not a guess to make
			"amp_attack=1.0,=0.2", // one good pair, one broken
		} {
			if _, err := parseOverrideList(in); err == nil {
				t.Errorf("parseOverrideList(%q) = nil error, want a rejection", in)
			}
		}
	})
}

// The render path DROPS an unknown key silently, so without this a typo measures the shipped sound as the candidate.
func TestValidateOverrideKeys(t *testing.T) {
	t.Run("nil and empty are accepted", func(t *testing.T) {
		if err := validateOverrideKeys(roster.Oboe, nil); err != nil {
			t.Errorf("nil overrides: %v", err)
		}
		if err := validateOverrideKeys(roster.Oboe, map[string]float64{}); err != nil {
			t.Errorf("empty overrides: %v", err)
		}
	})

	t.Run("real recipe parameters pass", func(t *testing.T) {
		if err := validateOverrideKeys(roster.Oboe, map[string]float64{"amp_attack": 0.05, "gen1_gain": 0.2}); err != nil {
			t.Errorf("known keys rejected: %v", err)
		}
	})

	t.Run("unknown keys are rejected and named", func(t *testing.T) {
		err := validateOverrideKeys(roster.Oboe, map[string]float64{
			"amp_attack":     0.05,
			"snare_wire_mix": 1.0, // a real param, but of a DIFFERENT recipe
			"nope_not_real":  1.0,
		})
		if err == nil {
			t.Fatal("unknown override keys accepted")
		}
		for _, want := range []string{"snare_wire_mix", "nope_not_real"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not name %q", err, want)
			}
		}
		if strings.Contains(err.Error(), "amp_attack") {
			t.Errorf("error %q names the VALID key amp_attack", err)
		}
	})
}
