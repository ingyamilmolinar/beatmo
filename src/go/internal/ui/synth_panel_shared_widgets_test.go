//go:build test

package ui

import (
	"testing"
)

// TestSynthPanel_FooterUsesSharedButtonChrome guards the discipline: every
// footer button (Reset / Save / Save As) must render via Button.Draw through
// either Style or SpecID. Bespoke drawRoundedRect chrome is not allowed.
//
// See feedback_shared_widget_reuse memory: clickables must reuse *Button so
// visuals + press/hover + spec-driven theming stay consistent across the app.
func TestSynthPanel_FooterUsesSharedButtonChrome(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	btns := env.g.drum.SynthTabButtons()
	if len(btns) == 0 {
		t.Fatal("no synth tab buttons present")
	}
	for _, b := range btns {
		if b == nil {
			continue
		}
		if !b.HasSpecID() && b.Style == nil {
			t.Errorf("synth footer button %q has neither SpecID nor Style — bespoke chrome (drawRoundedRect) instead of Button.Draw. Use NewSpecButton.",
				btnLabel(b.Text))
		}
	}
}

// TestSynthPanel_HeaderHasNoExtraButtons guards the explicit user
// direction to remove the "Preview on play" retrigger pill and the
// "vol N.NN" readout from the synth header. The row rack owns the
// per-row volume slider and the transport bar owns Play, so duplicating
// those affordances on the synth panel is forbidden.
func TestSynthPanel_HeaderHasNoExtraButtons(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	hdr := env.g.drum.SynthTabHeaderButtons()
	if len(hdr) != 0 {
		labels := make([]string, 0, len(hdr))
		for _, b := range hdr {
			if b != nil {
				labels = append(labels, b.Text)
			}
		}
		t.Errorf("synth header still carries %d button(s) %q — retrigger/vol pills must stay removed", len(hdr), labels)
	}
}
