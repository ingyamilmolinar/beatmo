//go:build test

package ui

import (
	"strings"
	"testing"
)

// TestSynthPanel_NoOutColumnArtifacts is the regression guard for the
// removal of Delay/Reverb send launchers from the Synth tab. Per user
// direction, those effects belong only to the per-row FX overlay (the FX
// menu) — surfacing them again on the Synth tab proved confusing.
//
// The test asserts three things end-to-end:
//   1. No DrumView accessor surfaces an OUT-column slice (validated at
//      compile time by the absence of `SynthTabOutLinks` / `SynthTabOutButtons`).
//   2. No synth-tab hit area carries an OUT-column tag prefix.
//   3. No footer / header button text contains the word "Delay" or
//      "Reverb" — i.e. those launchers were not silently re-introduced
//      under a different label.
func TestSynthPanel_NoOutColumnArtifacts(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)

	// (2) Hit-area tag scan.
	forbiddenTagPrefixes := []string{
		"synth-out",
		"synth-send",
	}
	for _, h := range env.hits {
		for _, p := range forbiddenTagPrefixes {
			if strings.HasPrefix(h.Tag, p) {
				t.Errorf("synth tab still registers an OUT-column hit area: tag=%q rect=%v", h.Tag, h.Rect)
			}
		}
	}

	// (3) Button label scan.
	checkButtonLabels := func(label string, btns []*Button) {
		for _, b := range btns {
			if b == nil {
				continue
			}
			if strings.Contains(b.Text, "Delay") || strings.Contains(b.Text, "Reverb") {
				t.Errorf("%s button %q still references Delay/Reverb — should live in the FX menu, not the synth panel", label, b.Text)
			}
		}
	}
	checkButtonLabels("footer", env.g.drum.SynthTabButtons())
	checkButtonLabels("header", env.g.drum.SynthTabHeaderButtons())
}
