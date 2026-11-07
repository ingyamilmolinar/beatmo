//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every ParamDef resolves to a non-empty name that is not its raw snake_case id.
func TestSynthParamDisplayName_EveryReachableParamResolves(t *testing.T) {
	seen := map[string]bool{}
	checked := 0
	for _, id := range []instrument.ID{roster.Modular} {
		for _, d := range audio.InstrumentParamDefs(id) {
			if seen[d.Name] {
				continue
			}
			seen[d.Name] = true
			checked++
			name := synthParamDisplayName(d.Name)
			if name == "" {
				t.Errorf("instrument %q param %q: display name is empty", id, d.Name)
				continue
			}
			if name == d.Name && strings.Contains(d.Name, "_") {
				t.Errorf("instrument %q param %q: display name leaks the raw snake_case id (%q)", id, d.Name, name)
			}
			// The prettifier replaces underscores and the override table never uses them.
			if strings.Contains(name, "_") {
				t.Errorf("instrument %q param %q: display name %q still contains an underscore", id, d.Name, name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("enumerated zero params")
	}
	t.Logf("resolved %d unique params to display names", checked)
}

func TestSynthParamDisplayName_KnownOverrides(t *testing.T) {
	cases := map[string]string{
		"osc_detune":       "Detune",
		"osc_octave":       "Octave",
		"amp_attack":       "Attack",
		"amp_decay":        "Decay",
		"amp_sustain":      "Sustain",
		"amp_release":      "Release",
		"filter_cutoff":    "Cutoff",
		"filter_resonance": "Resonance",
		"fm_op1_ratio":     "Op1 Ratio",
		"kick_click":       "Click",
		"noise_seed":       "Seed",
		// Two decays: the generic post `decay` is a multiplier, amp_decay the envelope seconds.
		"decay": "Decay ×",
	}
	for id, want := range cases {
		if got := synthParamDisplayName(id); got != want {
			t.Errorf("synthParamDisplayName(%q) = %q, want %q", id, got, want)
		}
	}
}

// genN_ and family prefixes come out prefix-stripped, underscore-free, Title-Cased.
func TestSynthParamDisplayName_FallbackPrettifier(t *testing.T) {
	cases := map[string]string{
		"gen1_kick_click":  "Kick Click",
		"gen10_snare_tune": "Snare Tune",
		"gen12_fm_r1":      "Fm R1",
		"tom_room":         "Room",
		"snare_attack":     "Attack",
		"post_decay_rate":  "Decay Rate",
	}
	for id, want := range cases {
		if got := prettifySynthParamID(id); got != want {
			t.Errorf("prettifySynthParamID(%q) = %q, want %q", id, got, want)
		}
	}
}

// SI Hz, seconds, signed cents/st/dB, multiplier, percent, unitless.
func TestFormatParamValue(t *testing.T) {
	cases := []struct {
		value float64
		unit  string
		want  string
	}{
		// Hz with SI prefix above 1000, integer below, no trailing ".00".
		{8000, "Hz", "8.0 kHz"},
		{1000, "Hz", "1.0 kHz"},
		{62, "Hz", "62 Hz"},
		{20, "Hz", "20 Hz"},
		{440.4, "Hz", "440 Hz"},
		// seconds, two decimals.
		{0.30, "s", "0.30 s"},
		{1.5, "s", "1.50 s"},
		// cents, signed integer.
		{12, "cents", "+12 c"},
		{0, "cents", "+0 c"},
		{-5, "cents", "-5 c"},
		{12, "c", "+12 c"},
		// semitones, signed integer.
		{3, "st", "+3 st"},
		{0, "st", "+0 st"},
		{-12, "st", "-12 st"},
		// dB, signed integer.
		{0, "dB", "+0 dB"},
		{-6, "dB", "-6 dB"},
		// multiplier, one decimal.
		{1.0, "×", "1.0×"},
		{2.5, "x", "2.5×"},
		// percent, integer.
		{40, "%", "40%"},
		// unitless, one decimal.
		{0.4, "", "0.4"},
		{1, "", "1.0"},
	}
	for _, c := range cases {
		if got := formatParamValue(c.value, c.unit); got != c.want {
			t.Errorf("formatParamValue(%v, %q) = %q, want %q", c.value, c.unit, got, c.want)
		}
	}
}

// Their unit is not in ParamDef.Unit: decay ×, drive/body/bright %, pitch st.
func TestFormatSynthParamValue_GenericPostControls(t *testing.T) {
	cases := []struct {
		name  string
		value float64
		want  string
	}{
		{"decay", 1.0, "1.0×"},
		{"drive", 0.4, "40%"},
		{"body", 0.5, "50%"},
		{"brightness", 0.75, "75%"},
		{"pitch", 3, "+3 st"},
	}
	for _, c := range cases {
		def := audio.ParamDef{Name: c.name}
		if got := formatSynthParamValue(def, c.value); got != c.want {
			t.Errorf("formatSynthParamValue(%q, %v) = %q, want %q", c.name, c.value, got, c.want)
		}
	}
}
