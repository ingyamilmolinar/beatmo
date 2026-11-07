//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Each sax arg must be a settable param that changes the render off its default; at-default identity is the golden
// net's job.
func TestSaxParamsConfigurable(t *testing.T) {
	Reset()
	ResetInstruments()

	// The shipped sax is additive, so the physical model is forced; comparing two such renders isolates the param.
	render := func(name string, val float64) []float32 {
		ResetInstrumentParams(roster.Sax)
		SetInstrumentParam(roster.Sax, "osc_enabled", 1)
		SetInstrumentParam(roster.Sax, "osc_type", 11)
		SetInstrumentParam(roster.Sax, name, val)
		buf, _ := RenderInstrumentOneShotRaw(roster.Sax)
		ResetInstrumentParams(roster.Sax)
		return buf
	}

	// An unregistered or inert param collapses both renders to one.
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
