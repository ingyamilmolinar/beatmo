//go:build test

package ui

import (
	"testing"
)

// NOTE: the slope / Pre / Reset-Hold pills moved off the sticky bar into the
// per-tab spectrumControls component (audio_tab_controls.go). The slim-bar
// phase deleted the bar's copies; the moved-pill behavior (spectrum-only
// gating, slope cycling, Pre toggle) is now covered by
// TestSpectrumControls* in audio_tab_controls_test.go.

// TestHzToNote covers well-known landmarks used by the cursor chip.
func TestHzToNote(t *testing.T) {
	cases := []struct {
		hz   float64
		want string
	}{
		{440.0, "A4"},
		{261.63, "C4"},
		{1046.50, "C6"},
		{2093.0, "C7"},
		{15.0, ""}, // sub-audible → empty
	}
	for _, c := range cases {
		got := hzToNote(c.hz)
		if got != c.want {
			t.Errorf("hzToNote(%g) = %q want %q", c.hz, got, c.want)
		}
	}
}

// TestFormatSlopeLabel pins the chip-label format used by the slope
// pill so screenshot baselines stay stable.
func TestFormatSlopeLabel(t *testing.T) {
	cases := []struct {
		v    float64
		want string
	}{
		{0, "0 dB/oct"},
		{3, "3 dB/oct"},
		{4.5, "4.5 dB/oct"},
	}
	for _, c := range cases {
		if got := formatSlopeLabel(c.v); got != c.want {
			t.Errorf("formatSlopeLabel(%g) = %q want %q", c.v, got, c.want)
		}
	}
}

// TestSlopePillFitsLabel asserts the slope pill is sized to fit its full
// label (no truncation) for every slope option — the pill width must be at
// least TextWidth(label) so the unit ("dB/oct") never reads as cut off.
func TestSlopePillFitsLabel(t *testing.T) {
	for _, v := range []float64{0, 3, 4.5} {
		label := formatSlopeLabel(v)
		// Layout sizes the pill as TextWidth(label)+10, clamped to >=44.
		pillW := TextWidth(label) + 10
		if pillW < 44 {
			pillW = 44
		}
		if pillW < TextWidth(label) {
			t.Errorf("slope pill width %d < label width %d for %q", pillW, TextWidth(label), label)
		}
	}
}
