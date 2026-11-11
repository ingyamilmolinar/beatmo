//go:build test

package ui

import "testing"

// sampler_toggle_state_test.go — every boolean toggle in the Sampler panel
// (Reverse, Normalize, Fade) must reflect its on/off state via the shared
// latched-keycap visual (Button.Toggled → amber engaged cap), identically on
// every platform. Previously Reverse showed no state and Normalize/Fade only
// swapped a spec fill, so toggled state read inconsistently or not at all.

func samplerWithBuffer() *DrumView {
	dv := &DrumView{}
	dv.sampler.raw = []float32{0.1, 0.2, 0.3, 0.4}
	dv.sampler.rawSampleRate = 48000
	dv.buildSamplerButtons(true)
	return dv
}

// TestSamplerTogglesLatchWhenOn pins that each toggle's latched visual follows
// its live predicate after a refresh.
func TestSamplerTogglesLatchWhenOn(t *testing.T) {
	assertDefaultParityState(t)
	cases := []struct {
		tag string
		set func(*DrumView, bool)
	}{
		{"sampler-reverse", func(dv *DrumView, on bool) { dv.sampler.reverse = on }},
		{"sampler-normalize", func(dv *DrumView, on bool) { dv.sampler.normalize = on }},
		{"sampler-fade", func(dv *DrumView, on bool) { dv.sampler.fadeOn = on }},
	}
	for _, c := range cases {
		dv := samplerWithBuffer()
		c.set(dv, true)
		dv.refreshSamplerToggleStates()
		if b := dv.samplerButtonByTag(c.tag); b == nil || !b.Toggled() {
			t.Errorf("%s: expected Toggled()=true when on", c.tag)
		}
		c.set(dv, false)
		dv.refreshSamplerToggleStates()
		if b := dv.samplerButtonByTag(c.tag); b == nil || b.Toggled() {
			t.Errorf("%s: expected Toggled()=false when off", c.tag)
		}
	}
}

// TestSamplerReverseIsToggle pins that Reverse is now a stateful toggle: it
// carries an active predicate and latches on after one click, off after two.
func TestSamplerReverseIsToggle(t *testing.T) {
	assertDefaultParityState(t)
	dv := samplerWithBuffer()
	var revDef *samplerButton
	for i := range dv.samplerButtons {
		if dv.samplerButtons[i].tag == "sampler-reverse" {
			revDef = &dv.samplerButtons[i]
		}
	}
	if revDef == nil {
		t.Fatal("sampler-reverse button not found")
	}
	if revDef.active == nil {
		t.Fatal("Reverse must be a toggle (active predicate non-nil) so it shows its reversed state")
	}
	if dv.samplerButtonByTag("sampler-reverse").Toggled() {
		t.Fatal("reverse should start un-latched")
	}
	dv.samplerButtonByTag("sampler-reverse").OnClick()
	dv.refreshSamplerToggleStates()
	if !dv.samplerButtonByTag("sampler-reverse").Toggled() {
		t.Error("reverse should latch ON after one click")
	}
	dv.samplerButtonByTag("sampler-reverse").OnClick()
	dv.refreshSamplerToggleStates()
	if dv.samplerButtonByTag("sampler-reverse").Toggled() {
		t.Error("reverse should latch OFF after a second click")
	}
}

// TestSamplerTogglesNeverLatchWithoutBuffer pins that with no buffer loaded the
// toggles never present a latched (active) affordance even if a stale predicate
// would read true — the empty panel shows no live state.
func TestSamplerTogglesNeverLatchWithoutBuffer(t *testing.T) {
	assertDefaultParityState(t)
	dv := &DrumView{}
	dv.buildSamplerButtons(false) // no buffer
	dv.sampler.normalize = true
	dv.refreshSamplerToggleStates()
	if b := dv.samplerButtonByTag("sampler-normalize"); b != nil && b.Toggled() {
		t.Error("normalize must not latch while no buffer is loaded")
	}
}
