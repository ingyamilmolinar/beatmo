package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Every knob the Synth tab shows carries a plain-English gloss so a non-expert understands it.
func TestP4_EveryExposedParamHasPlainEnglish(t *testing.T) {
	missing := map[string]string{}
	for _, d := range audio.ModularSynthParamDefs() {
		if d.Group == audio.SynthHiddenGroup {
			continue
		}
		if PlainEnglish(d.Label) == "" {
			missing[d.Name] = d.Label
		}
	}
	if len(missing) > 0 {
		for name, label := range missing {
			t.Errorf("exposed synth param %q (label %q) has no PlainEnglish gloss — add it to plainEnglishMap", name, label)
		}
	}
}
