//go:build test

package ui

import (
	"strings"
	"testing"
)

// Delay/Reverb belong only to the row's FX panel: no OUT-column accessor, hit-area tag or Delay/Reverb button
// text on the Synth tab.
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
