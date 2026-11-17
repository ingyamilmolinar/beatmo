package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestP4_EveryExposedParamHasPlainEnglish is the P4 understanding guard: every
// knob a user can see in the Synth tab must carry a plain-English gloss so a
// non-expert understands what it does. "Exposed" = a ModularSynthParamDef NOT in
// the hidden group. If a new exposed param is added without a gloss, this fails
// and points at the missing label.
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
