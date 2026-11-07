//go:build test

package ui

import (
	"testing"
)

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
