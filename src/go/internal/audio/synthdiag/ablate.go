//go:build !test && !js

package synthdiag

import (
	"fmt"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// The test/js build refuses instead: a stand-in synth's stage leverage would describe the stub, not the engine.

func rms(w wave.Wave) float64 {
	var s float64
	for _, v := range w.Samples {
		s += v * v
	}
	if len(w.Samples) == 0 {
		return 0
	}
	return math.Sqrt(s / float64(len(w.Samples)))
}

// Without it a stage that only changes LEVEL would top every level-sensitive axis, the same reason `refs compare`
// level-matches.
func matchRMS(target, w wave.Wave) wave.Wave {
	tr, wr := rms(target), rms(w)
	if wr <= 1e-9 {
		return w
	}
	g := tr / wr
	out := wave.Wave{Samples: make([]float64, len(w.Samples)), SampleRate: w.SampleRate, Label: w.Label}
	for i, v := range w.Samples {
		out.Samples[i] = v * g
	}
	return out
}

// Each active stage at identity, RMS-matched, measured above its no-effect floor. Sensitivity, not contribution:
// stages interact, so rows do not sum. base is the diagnosis's own override map, so the full render is the one the
// report describes.
func Ablate(instID instrument.ID, pitch float64, sr int, durSec float64, base map[string]float64) (Report, error) {
	d, err := audio.InstrumentDocWithFields(instID, base)
	if err != nil {
		return Report{}, fmt.Errorf("synthdiag: %w", err)
	}
	merged := instrument.FieldValues(d, audio.BuiltinModuleRegistry())
	full, err := synthmatch.RenderWithParams(instID, pitch, sr, durSec, base)
	if err != nil {
		return Report{}, err
	}
	fullB := fingerprint.NewBundle(full, string(instID))
	descs := fingerprint.DefaultRegistry().Applicable(fullB)
	rep := Report{Instrument: instID}
	fullM := make([]fingerprint.Measurement, len(descs))
	for i, d := range descs {
		rep.Axes = append(rep.Axes, d.ID)
		rep.Names = append(rep.Names, d.Name)
		fullM[i] = d.Extract(fullB)
	}
	for _, st := range ActiveStages(map[string]float64(merged)) {
		ov := map[string]float64{}
		for k, v := range base {
			ov[k] = v
		}
		for k, v := range st.Overrides {
			ov[k] = v
		}
		w, err := synthmatch.RenderWithParams(instID, pitch, sr, durSec, ov)
		if err != nil {
			return Report{}, fmt.Errorf("synthdiag: ablate %s: %w", st.Name, err)
		}
		b := fingerprint.NewBundle(matchRMS(full, w), string(instID)+"-"+st.Name)
		row := make([]float64, len(descs))
		for i, d := range descs {
			v, ok := stageSensitivity(d, fullM[i], d.Extract(b))
			if !ok {
				row[i] = math.NaN()
				continue
			}
			row[i] = v
		}
		rep.Stages = append(rep.Stages, st.Name)
		rep.Sens = append(rep.Sens, row)
	}
	return rep, nil
}
