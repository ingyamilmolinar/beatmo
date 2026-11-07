// WHICH stage to reach for, by ablation: re-render with each active stage at identity and measure how far each
// descriptor moves. A SENSITIVITY matrix, not a decomposition: stages interact, so the numbers never sum to shares.
package synthdiag

import (
	"fmt"
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Some stages are identity BY VALUE. A gen slot is ablated by source 0, not gain 0, which several sources read as
// "unconfigured" rather than silence.

// Ablating an already-off stage costs a render for a guaranteed zero row.
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

func ActiveStages(merged map[string]float64) []Stage {
	var out []Stage
	for _, s := range Stages() {
		if s.Active(merged) {
			out = append(out, s)
		}
	}
	return out
}

// Not zero for every Kind: Seconds and Ratio fold to ≥ 1 before dividing by the mismatch, so a self-comparison reads
// 1/mismatch. Unsubtracted, the matrix stops being comparable across axes.
func noEffectFloor(d fingerprint.Descriptor, full fingerprint.Measurement) float64 {
	self := d.CompareWith(full, full)
	if !self.OK {
		return 0
	}
	return self.Norm
}

// ok=false = not comparable (stored NaN, dropped rather than ranked). Clamped at 0: below the floor is noise.
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

// RMS-matched minus the no-effect floor, so 0 means "changed nothing" on every axis; NaN = not comparable. A stage can
// score on an axis it does not own by changing what a later stage receives.
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

// STABLE: ties are common (a segment boundary shift moves several axes equally), and a report reordering its own top 3
// reads as measurement instability.
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
