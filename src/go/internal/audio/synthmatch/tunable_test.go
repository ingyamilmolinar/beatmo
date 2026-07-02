package synthmatch

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestSpecKeysExistInRecipe(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	for _, inst := range []string{"sax", "organ", "guitar-electric", "violin", "trumpet", "piano-grand"} {
		spec, ok := SpecForInstrument(inst)
		if !ok {
			t.Fatalf("no TunableSpec for %q", inst)
		}
		if err := spec.ValidateKeys(inst); err != nil {
			t.Errorf("%s: %v", inst, err)
		}
	}
}

func TestRenderWithParams_ChangesOutput(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	base, err := RenderInstrument("sax", 0, 44100, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	mod, err := RenderWithParams("sax", 0, 44100, 0.5, map[string]float64{"filter_cutoff": 400})
	if err != nil {
		t.Fatal(err)
	}
	if len(base.Samples) > 1000 && len(mod.Samples) > 1000 &&
		base.Samples[1000] == mod.Samples[1000] && base.PeakSample() == mod.PeakSample() {
		t.Error("filter_cutoff override did not change output")
	}
}
