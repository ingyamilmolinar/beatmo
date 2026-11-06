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

// ablate.go — the real (CGo synth) ablation runner. The `test`/`js` build gets
// ablate_stub.go instead: a pure-Go stand-in synth would make every stage's
// leverage a property of the stub rather than of the shipped engine, which is
// worse than refusing to answer.

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

// matchRMS scales w to target's RMS. Without it a stage that only changes
// LEVEL (a gen slot carrying most of the energy, the post gain trim) would
// move every level-sensitive descriptor and be reported as the top driver of
// axes it has no timbral bearing on -- the same reason `refs compare`
// level-matches before a listener judges.
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

// Ablate renders instID in full, then once per active stage with that stage
// at identity, RMS-matches each ablation to the full render and reports how
// far each applicable descriptor moved ABOVE its no-effect floor (see
// stageSensitivity -- the raw Norm of a seconds axis compared with itself is
// 1/mismatch, not 0). Sensitivity, not contribution: stages interact, so the
// numbers do not sum.
//
// base is the same override map the diagnosis was rendered with (nil for a
// shipped instrument), so the full render here is byte-identical to the one
// the report describes; each stage's overrides are layered ON TOP of it.
//
// Applicability is evaluated against the FULL render (there is no reference
// here -- this compares a synth against itself), so the axis set is exactly
// the one the full sound supports.
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
