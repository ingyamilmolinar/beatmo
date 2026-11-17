package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// synth_modal_demo.go — a fixed document shaped like a WAV clone (MODAL +
// STRIKE + NOISE stages, ratio rows, a measured-looking decay law) for the
// Synth-tab scenes and tests of those stages. Generated, never measured: the
// real clones come from `make clone` and are local dev artifacts; nothing here
// reads reference audio.

// demoModalRatios are a free-free bar's partial ratios (and two doublets).
var demoModalRatios = []float64{1, 2.756, 2.764, 5.404, 8.933, 8.951, 13.345, 18.64, 24.81, 31.87, 39.8, 48.6, 58.2, 68.8}

// demoModalDoc replaces base's sound with the demo modal model, keeping its
// identity (id, name, category) like `clone -into`.
func demoModalDoc(base instrument.Doc) (instrument.Doc, error) {
	reg := audio.BuiltinModuleRegistry()
	d := instrument.Doc{ID: base.ID, Origin: base.Origin, Name: base.Name, Category: base.Category,
		Render: instrument.Render{Seconds: 3, Amplitude: math.Max(base.Render.Amplitude, 0.5), Melodic: true},
		Stages: map[string]instrument.Stage{}}
	var err error
	for _, typ := range []string{"core", "finish", "modal", "strike", "noiseband"} {
		if d, err = instrument.AddStage(d, reg, typ, true); err != nil {
			return d, err
		}
	}
	mv := d.Stages["modal"].Values
	mv["modal_freq_mode"], mv["modal_law_a"], mv["modal_law_p"], mv["modal_law_b"] = 0, 1.4, 0.85, 20
	mv["modal_keep"] = float64(len(demoModalRatios))
	sv := d.Stages["strike"].Values
	sv["strike_tc"], sv["strike_hardness"] = 0.0006, -1.5
	mt := instrument.Table{Cols: []string{"freq", "decay", "amp", "phase", "attack"}}
	for i, r := range demoModalRatios {
		amp := 0.5 / math.Pow(float64(i+1), 0.7)
		mt.Rows = append(mt.Rows, []float64{r, 0.8 + 0.05*float64(i%4), amp, math.Remainder(1.3*float64(i), 2*math.Pi), 0})
	}
	nt := instrument.Table{Cols: []string{"center", "q", "level", "attack", "fast_rate", "fast_mix", "tail_rate"}}
	for k := 0; k < 6; k++ {
		nt.Rows = append(nt.Rows, []float64{400 * math.Pow(2, float64(k)), 4.318, -38 - 3*float64(k), 0.002, 120, 0.8, 12 + 4*float64(k)})
	}
	if d, err = instrument.SetTable(d, reg, "modal", mt); err != nil {
		return d, err
	}
	if d, err = instrument.SetTable(d, reg, "noiseband", nt); err != nil {
		return d, err
	}
	return d, instrument.Validate(d, reg)
}

// installDemoModalDoc makes id's project document the demo modal model.
func installDemoModalDoc(id instrument.ID) error {
	base, ok := audio.EnsureInstrumentDoc(id)
	if !ok {
		base = instrument.Doc{ID: id, Name: string(id), Category: "mallet"}
	}
	d, err := demoModalDoc(base)
	if err != nil {
		return err
	}
	return audio.SetInstrumentDoc(id, d)
}
