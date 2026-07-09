//go:build !test && !js

package audio

import "testing"

// TestSaxParamsConfigurable is the TDD driver for P2 (physical-model config): the
// sax's previously-hardcoded render_sax args (blow position, reed offset/slope,
// bell reflection, breath, loss) become declarative config params. Each must
// (a) be a registered, settable param and (b) actually change the render when
// moved off its default. The AT-DEFAULT byte-identity is covered by
// TestSynthGoldenByteIdentity (the golden net); this test covers MUTATION.
func TestSaxParamsConfigurable(t *testing.T) {
	Reset()
	ResetInstruments()

	// Render the sax with one param set to `val`, FORCING the render_sax physical
	// model (osc_type 11): the shipped "sax" instrument is now an additive voice
	// (osc_enabled 0), so we override osc_type/osc_enabled to exercise the
	// config-driven render_sax path that P2 wired. Comparing two of these isolates
	// the PARAM from any baked→recipe path switch.
	render := func(name string, val float64) []float32 {
		ResetInstrumentParams("sax")
		SetInstrumentParam("sax", "osc_enabled", 1)
		SetInstrumentParam("sax", "osc_type", 11)
		SetInstrumentParam("sax", name, val)
		buf, _ := RenderInstrumentOneShotRaw("sax")
		ResetInstrumentParams("sax")
		return buf
	}

	// Each new sax knob: its shipped default vs a value clearly off it. If the
	// param is wired through to render_sax, the two renders differ; if it is
	// unregistered/inert, both collapse to the same render (test stays red).
	cases := []struct {
		name          string
		def, modified float64
	}{
		{"osc_sax_blow", 0.15, 0.35},
		{"osc_sax_reed_off", 0.58, 0.40},
		{"osc_sax_reed_slope", 0.28, 0.50},
		{"osc_sax_reflect", -0.94, -0.80},
		{"osc_sax_breath", 0.85, 0.50},
		{"osc_sax_loss", 0.70, 0.50},
	}
	for _, c := range cases {
		a := render(c.name, c.def)
		b := render(c.name, c.modified)
		if len(a) == 0 || buffersEqual(a, b) {
			t.Errorf("%s: default=%v vs %v produced the SAME render — not wired through to render_sax", c.name, c.def, c.modified)
		}
	}
}

func buffersEqual(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
