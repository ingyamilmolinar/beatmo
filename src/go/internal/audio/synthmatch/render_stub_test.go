//go:build test

package synthmatch

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func differs(a, b []float64) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

func TestRenderStub_RespondsToMultipleParams(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	base, err := RenderInstrument("sax", 0, 44100, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"filter_cutoff", "amp_attack", "amp_sustain", "lfo_depth"} {
		var val float64
		switch key {
		case "filter_cutoff":
			val = 600
		case "amp_attack":
			val = 0.09
		case "amp_sustain":
			val = 0.2
		case "lfo_depth":
			val = 0.18
		}
		mod, err := RenderWithParams("sax", 0, 44100, 0.5, map[string]float64{key: val})
		if err != nil {
			t.Fatal(err)
		}
		if !differs(base.Samples, mod.Samples) {
			t.Errorf("test renderer is BLIND to %q — optimizer cannot descend on it", key)
		}
	}
}
