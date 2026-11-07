//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Precision and unit come from the definition (Min/Max/Unit), never the value: kHz from 1000 Hz, ms/st/% integer,
// sub-unit ranges two decimals.
func TestFXFormatParamRendersUnitsAndPrecision(t *testing.T) {
	cases := []struct {
		name string
		def  audio.EffectParamDef
		val  float64
		want string
	}{
		// time: wide integer-domain ms range — no fake decimal.
		{"time-250", audio.EffectParamDef{Min: 10, Max: 1000, Unit: "ms"}, 250, "250 ms"},
		// mix: 0..1 range → two decimals, ALWAYS (this is the anti-jitter case).
		{"mix-1", audio.EffectParamDef{Min: 0, Max: 1, Unit: ""}, 1, "1.00"},
		{"mix-0.3", audio.EffectParamDef{Min: 0, Max: 1, Unit: ""}, 0.3, "0.30"},
		// feedback: 0..0.95 range → two decimals.
		{"feedback-0.4", audio.EffectParamDef{Min: 0, Max: 0.95, Unit: ""}, 0.4, "0.40"},
		// drive: 1..20 range → one decimal, stable.
		{"drive-2", audio.EffectParamDef{Min: 1, Max: 20, Unit: ""}, 2, "2.0"},
		{"drive-8.2", audio.EffectParamDef{Min: 1, Max: 20, Unit: ""}, 8.2, "8.2"},
		// tone: audio frequency (Max >= 1000) → integer Hz, kHz above 1000.
		{"tone-4000", audio.EffectParamDef{Min: 200, Max: 8000, Unit: "Hz"}, 4000, "4.0 kHz"},
		{"tone-800", audio.EffectParamDef{Min: 200, Max: 8000, Unit: "Hz"}, 800, "800 Hz"},
		// cutoff: audio frequency at exactly 1000 → kHz boundary.
		{"cutoff-1000", audio.EffectParamDef{Min: 20, Max: 20000, Unit: "Hz"}, 1000, "1.0 kHz"},
		{"cutoff-440", audio.EffectParamDef{Min: 20, Max: 20000, Unit: "Hz"}, 440, "440 Hz"},
		// LFO rate: low-frequency Hz (Max < 1000) → decimal Hz, not integer.
		{"rate-1.5", audio.EffectParamDef{Min: 0.1, Max: 10, Unit: "Hz"}, 1.5, "1.5 Hz"},
		// dB: one decimal (ceiling default is -0.3).
		{"ceiling", audio.EffectParamDef{Min: -6, Max: 0, Unit: "dB"}, -0.3, "-0.3 dB"},
		// percent: integer, glued to the sign (no space).
		{"attack-100", audio.EffectParamDef{Min: 0, Max: 200, Unit: "%"}, 100, "100%"},
		// semitones: integer.
		{"pitch-neg", audio.EffectParamDef{Min: -24, Max: 24, Unit: "st"}, -12, "-12 st"},
		{"pitch-zero", audio.EffectParamDef{Min: -24, Max: 24, Unit: "st"}, 0, "0 st"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fxFormatParam(tc.def, tc.val)
			if got != tc.want {
				t.Errorf("fxFormatParam(%+v, %v) = %q, want %q", tc.def, tc.val, got, tc.want)
			}
		})
	}
}

// Fractional digits are constant across every param's range, or the label reflows during a drag.
func TestFXFormatParamPrecisionIsStablePerParam(t *testing.T) {
	fracDigits := func(s string) int {
		// Strip trailing unit (e.g. " kHz", " ms", "%").
		s = strings.TrimRight(s, "%")
		if i := strings.IndexByte(s, ' '); i >= 0 {
			s = s[:i]
		}
		if dot := strings.IndexByte(s, '.'); dot >= 0 {
			return len(s) - dot - 1
		}
		return 0
	}
	for effType, defs := range audio.InsertEffectCatalog() {
		for _, def := range defs {
			def := def
			// Sample the range densely, including both endpoints.
			const steps = 40
			want := -1
			for i := 0; i <= steps; i++ {
				v := def.Min + (def.Max-def.Min)*float64(i)/float64(steps)
				got := fxFormatParam(def, v)
				d := fracDigits(got)
				// The kHz rollover legitimately changes the digit count.
				if def.Unit == "Hz" && def.Max >= 1000 {
					continue
				}
				if want < 0 {
					want = d
					continue
				}
				if d != want {
					t.Errorf("%s.%s: fractional digits jitter — %q has %d, earlier had %d",
						effType, def.Name, got, d, want)
					break
				}
			}
		}
	}
}
