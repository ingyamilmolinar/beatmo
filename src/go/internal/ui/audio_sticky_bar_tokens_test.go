//go:build test

package ui

import "testing"

// TestAudioStickyBarTokens pins the audio-panel sticky-bar pill sizing to
// density tokens so the tab chrome is re-styleable from DESIGN.md (was ~20
// hand-tuned literals). Comfortable preserves today's values.
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
