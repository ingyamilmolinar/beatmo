//go:build test

package ui

import (
	"testing"
)

// Through Button.Draw (Style or SpecID), never bespoke chrome, so visuals, press/hover and theming stay shared.
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

// No retrigger pill or volume readout: the row rack and transport own those.
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
