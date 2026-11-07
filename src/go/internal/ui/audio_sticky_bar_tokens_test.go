//go:build test

package ui

import "testing"

// Density tokens, so the chrome restyles from DESIGN.md; Comfortable keeps the shipped values.
func TestAudioStickyBarTokens(t *testing.T) {
	d := Profile().DensityValues()
	checks := map[string]int{
		"AudioPillH":       d.AudioPillH,
		"AudioPillGap":     d.AudioPillGap,
		"AudioPillPadX":    d.AudioPillPadX,
		"AudioPillNarrowW": d.AudioPillNarrowW,
		"AudioTabMinW":     d.AudioTabMinW,
		"AudioChannelMinW": d.AudioChannelMinW,
	}
	for name, v := range checks {
		if v <= 0 {
			t.Errorf("density token %s unset (got %d)", name, v)
		}
	}
}
