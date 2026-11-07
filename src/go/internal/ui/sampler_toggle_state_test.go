//go:build test

package ui

import "testing"

// Reverse, Normalize and Fade show their state through the shared latched keycap, on every platform.

func samplerWithBuffer() *DrumView {
	dv := &DrumView{}
	dv.sampler.raw = []float32{0.1, 0.2, 0.3, 0.4}
	dv.sampler.rawSampleRate = 48000
	dv.buildSamplerButtons(true)
	return dv
}

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

// Latched after one click, released after two.
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

// The empty panel shows no live state, whatever a stale predicate reads.
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
