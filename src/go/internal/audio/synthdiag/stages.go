// Package synthdiag diagnoses WHICH PART of the synthesis chain is responsible
// for a measured reference-vs-synth mismatch.
//
// `refs diagnose` answers "what is wrong" per axis; this package answers "which
// knob group to reach for". It does so by ABLATION: render the instrument in
// full, then re-render it once per active stage with that stage held at
// identity, and measure how far each descriptor moves. The result is a
// SENSITIVITY matrix, not a decomposition -- stages interact (a filter's effect
// depends on what the oscillator fed it), so the per-stage numbers do not sum
// to the whole and must never be presented as shares of one.
package synthdiag

import (
	"fmt"
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// stages.go — what "this stage at identity" means, per synthesis stage.
//
// Several Synth-tab stages are identity BY VALUE, not by a flag: ENSEMBLE is
// unison_voices=1 with zero detune/drift, RESONATOR is body_mix=0, a gen
// slot is genN_source=0. A gen slot is deliberately NOT ablated through
// genN_gain: for the transplanted sources (KS, analytic, kick, tom, snare,
// clap, cymbal, FM) gain 0 means "unconfigured", so it would not silence
// them; source 0 does.

// Stage is one ablatable synthesis stage: the override set that puts it at
// identity, plus the predicate that says whether it is doing anything at all
// in a given merged parameter set (ablating an already-off stage costs a
// render and yields a guaranteed zero row).
type Stage struct {
	Name      string
	Overrides map[string]float64
	Active    func(merged map[string]float64) bool
}

func flagOn(key string) func(map[string]float64) bool {
	return func(m map[string]float64) bool { return m[key] > 0.5 }
}

// Stages returns the ablation table in report order.
func Stages() []Stage {
	s := []Stage{
		{"osc", map[string]float64{"osc_enabled": 0}, flagOn("osc_enabled")},
	}
	for i := 1; i <= 8; i++ {
		src := fmt.Sprintf("gen%d_source", i)
		s = append(s, Stage{fmt.Sprintf("gen%d", i), map[string]float64{src: 0}, flagOn(src)})
	}
	s = append(s,
		Stage{"ensemble", map[string]float64{"unison_voices": 1, "unison_detune": 0, "unison_drift_rate": 0, "unison_drift_depth": 0},
			func(m map[string]float64) bool { return m["unison_voices"] > 1.5 }},
		Stage{"fm", map[string]float64{"fm_enabled": 0}, flagOn("fm_enabled")},
		Stage{"pitchenv", map[string]float64{"pitchenv_enabled": 0}, flagOn("pitchenv_enabled")},
		Stage{"lfo", map[string]float64{"lfo_enabled": 0}, flagOn("lfo_enabled")},
		Stage{"burst", map[string]float64{"burst_enabled": 0}, flagOn("burst_enabled")},
		Stage{"filter", map[string]float64{"filter_enabled": 0}, flagOn("filter_enabled")},
		Stage{"filtenv", map[string]float64{"filtenv_enabled": 0}, flagOn("filtenv_enabled")},
		Stage{"formant", map[string]float64{"formant_enabled": 0}, flagOn("formant_enabled")},
		Stage{"modal", map[string]float64{"modal_enabled": 0}, flagOn("modal_enabled")},
		Stage{"strike", map[string]float64{"strike_enabled": 0}, flagOn("strike_enabled")},
		Stage{"noiseband", map[string]float64{"noiseband_enabled": 0}, flagOn("noiseband_enabled")},
		Stage{"resonator", map[string]float64{"body_mix": 0}, func(m map[string]float64) bool { return m["body_mix"] > 0.01 }},
		Stage{"bow", map[string]float64{"bow_dynamics": 0}, func(m map[string]float64) bool { return m["bow_dynamics"] > 0.01 }},
		Stage{"drive", map[string]float64{"drive_enabled": 0}, flagOn("drive_enabled")},
		Stage{"post", map[string]float64{"post_pitch": 0, "post_decay": 1, "post_body": 0,
			"post_brightness": 0, "post_darken": 0, "post_drive": 0},
			func(m map[string]float64) bool {
				return m["post_pitch"] != 0 || (m["post_decay"] != 1 && m["post_decay"] != 0) || m["post_body"] > 0 ||
					m["post_brightness"] > 0 || m["post_darken"] > 0 || m["post_drive"] > 0
			}},
		Stage{"kick", map[string]float64{"kick_enabled": 0}, flagOn("kick_enabled")},
	)
	return s
}

// ActiveStages filters Stages() down to the ones actually doing something in
// merged.
func ActiveStages(merged map[string]float64) []Stage {
	var out []Stage
	for _, s := range Stages() {
		if s.Active(merged) {
			out = append(out, s)
		}
	}
	return out
}

// noEffectFloor is the Norm a descriptor reports when a measurement is
// compared with ITSELF -- the reading a stage with literally zero effect
// produces.
//
// It is NOT zero for every Kind. KindSeconds and KindRatio fold their ratio to
// >=1 before dividing by the mismatch threshold (compare.go's
// ratioNormAndDisplay), so an identical pair yields 1/mismatch: 0.50 at the
// default mismatch of 2.0, 0.36 on an axis calibrated to 2.75. The
// absolute-delta kinds floor at 0. Left unsubtracted, a stage that does
// nothing reads 0.50 on a seconds axis and 0.00 on a fraction axis, and the
// matrix stops being comparable across axes -- which is the one thing a
// sensitivity matrix is for.
func noEffectFloor(d fingerprint.Descriptor, full fingerprint.Measurement) float64 {
	self := d.CompareWith(full, full)
	if !self.OK {
		return 0
	}
	return self.Norm
}

// stageSensitivity is how far one ablation moved a descriptor ABOVE its
// no-effect floor. ok=false means the two renders were not comparable on this
// axis at all (the caller stores NaN, which TopStages drops rather than ranks
// as zero leverage).
//
// Clamped at 0: below-floor is not a meaningful direction here, it is
// arithmetic noise around "no effect".
func stageSensitivity(d fingerprint.Descriptor, full, ablated fingerprint.Measurement) (float64, bool) {
	delta := d.CompareWith(full, ablated)
	if !delta.OK {
		return 0, false
	}
	v := delta.Norm - noEffectFloor(d, full)
	if v < 0 {
		v = 0
	}
	return v, true
}

// Report is one instrument's stage x axis sensitivity matrix.
//
// Sens[stage][axis] is the descriptor's Delta.Norm between the FULL render and
// the render with that stage at identity (RMS-matched first, so a stage that
// merely changes level is not credited with changing timbre), MINUS that
// descriptor's no-effect floor (see noEffectFloor) so 0 means "this stage
// changed nothing" on every axis regardless of Kind. NaN means the axis was
// not comparable between the two renders. Rows are sensitivities, not
// contributions: they do not sum, and a stage can score high on an axis it
// does not "own" simply by changing what a later stage receives.
type Report struct {
	Instrument instrument.ID
	Stages     []string    // ablated stage names, in Stages() order
	Axes       []string    // descriptor IDs applicable to the full render
	Names      []string    // descriptor Names, same order
	Sens       [][]float64 // [stage][axis] Delta.Norm of (full vs ablated); NaN when not comparable
}

// StageSens is one (stage, sensitivity) pair for a single axis.
type StageSens struct {
	Stage string
	Norm  float64
}

// TopStages returns the n stages that move axisID the most, largest first.
// Axes the ablation could not compare are dropped rather than ranked as zero.
//
// The sort is STABLE, so equal sensitivities keep Stages() order rather than
// permuting between runs. Ties are common and not a corner case -- a stage
// that shifts a segment boundary moves several segment axes by exactly the
// same amount -- and an ablation report that reorders its own top-3 on a
// re-run reads as instability in the measurement rather than in the sort.
func (r Report) TopStages(axisID string, n int) []StageSens {
	col := -1
	for i, id := range r.Axes {
		if id == axisID {
			col = i
			break
		}
	}
	if col < 0 {
		return nil
	}
	var out []StageSens
	for si, name := range r.Stages {
		v := r.Sens[si][col]
		if !math.IsNaN(v) {
			out = append(out, StageSens{name, v})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Norm > out[j].Norm })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
