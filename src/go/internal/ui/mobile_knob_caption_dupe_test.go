//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Mobile knob cells render a value PILL with a caption band below it. The
// pill carries the VALUE; the caption must carry the parameter NAME — the
// two used to both render the same string, so every mobile knob showed its
// label twice ("Start 0%" / "Start 0%" on the Sampler, "Base" / "Base" on
// the Synth tab — A9 in the 2026-07-04 critique).

func TestSynthMobileKnobCaptionIsParamName(t *testing.T) {
	def := audio.ParamDef{Name: "wave", Min: 0, Max: 1, Enum: []string{"Base", "Saw"}}

	caption := synthKnobMobileCaption(def)
	pill := formatKnobDisplay(def, 0) // pill label at value "Base"

	if caption == pill {
		t.Fatalf("mobile caption %q duplicates the pill label — caption must be the param name", caption)
	}
	if want := synthParamDisplayName(def.Name); caption != want {
		t.Fatalf("mobile caption = %q, want param display name %q", caption, want)
	}
}

func TestSamplerMobilePillValueCaptionName(t *testing.T) {
	s := &samplerState{startFrac: 0.15}

	name := samplerKnobNameText(samplerKnobStart, s)
	value := samplerKnobValueText(samplerKnobStart, s)
	full := samplerKnobCaption(samplerKnobStart, s)

	if name == "" || value == "" {
		t.Fatalf("empty split: name=%q value=%q", name, value)
	}
	if name == full {
		t.Fatal("caption name equals the full name+value string — the mobile caption would duplicate the pill")
	}
	if want := name + " " + value; full != want {
		t.Fatalf("split drifted from the composed caption: %q + %q != %q", name, value, full)
	}
}
