//go:build !test && !js

package ui

import (
	"math"
	"sync/atomic"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// Through the real C voice at each dial's 0 and 100: some end changes the render, and a declared metric moves the
// declared way from the shipped sound toward each end.
func TestEveryDialMovesItsMetric(t *testing.T) {
	// The played voice, the sound a soloed row plays: the caps and gates are about it, and a raw 4 s render measures a
	// sound no trigger produces.
	g := buildTestGame(t)
	withPrivateInstrumentLibrary(t)
	audio.ResetInstrumentDocs()
	t.Cleanup(audio.ResetInstrumentDocs)
	audio.SetBPM(g.drum.BPM())
	audio.SetDispatchMustNotRender(false)
	t.Cleanup(func() { audio.SetDispatchMustNotRender(true) })
	sr := audio.SampleRate()
	render := func(id string, fields map[string]float64) []float32 {
		audio.ResetInstrumentParams(instrument.ID(id))
		if fields != nil {
			audio.SetInstrumentParamsLive(instrument.ID(id), fields)
		}
		x, _ := audio.RenderAsPlayed(instrument.ID(id), 0, 1, 4)
		audio.ResetInstrumentParams(instrument.ID(id))
		if len(x) == 0 {
			return nil
		}
		out := make([]float32, len(x))
		for i, v := range x {
			out[i] = float32(v)
		}
		return out
	}
	measure := func(id, metric string, buf []float32) (float64, bool) {
		d, ok := fingerprint.DefaultRegistry().ByID(metric)
		if !ok {
			return 0, false // TestSynthDialStringsAndMetricsExist owns this rule
		}
		// Converted to the rate the fingerprint descriptors were calibrated at: several count samples, so at 48 kHz a
		// metric can read backwards while the sound moves the right way.
		conv := audio.ResampleToRate(buf, sr, wavref.Rate)
		s := make([]float64, len(conv))
		for i, v := range conv {
			s[i] = float64(v)
		}
		b := fingerprint.NewBundle(wave.Wave{Samples: s, SampleRate: wavref.Rate, Label: id}, id)
		if d.Applies != nil && !d.Applies(b) {
			return 0, false
		}
		m := d.Extract(b)
		return m.Scalar, m.OK
	}

	var checked, inaudible atomic.Int64
	for _, id := range roster.All {
		schema := audio.InstrumentParamDefs(id)
		if len(schema) == 0 {
			continue
		}
		audio.ResetInstrumentParams(id)
		anchor := cloneRecipeParams(audio.MergedInstrumentParamsRO(id))
		dialMetricCheck(t, string(id), schema, anchor, g.drum.synthVoiceSecs(id), render, measure, &checked, &inaudible)
	}
	t.Logf("checked %d (instrument, dial) pairs; %d inaudible", checked.Load(), inaudible.Load())
	if checked.Load() < 300 {
		t.Errorf("only %d (instrument, dial) pairs checked — the roster walk is broken", checked.Load())
	}
}

func dialMetricCheck(t *testing.T, id string, schema []audio.ParamDef, anchor audio.RecipeParams, secs float64,
	render func(string, map[string]float64) []float32, measure func(string, string, []float32) (float64, bool),
	checked, inaudible *atomic.Int64) {
	{
		plain := render(id, nil)
		if plain == nil {
			t.Fatalf("%s: the instrument does not render", id)
		}
		// With the played voice's length, which the caps and the "settles" gate read.
		for _, g := range buildMacroGroupsForVoice(schema, anchor, secs) {
			for _, d := range g.Dials {
				r0 := synthDialPercent(d.Reading(anchor))
				lo, hi := render(id, d.Turn(anchor, 0)), render(id, d.Turn(anchor, 1))
				if lo == nil || hi == nil {
					t.Errorf("%s %s/%s: a dial end does not render", id, g.ID, d.Dial.ID)
					continue
				}
				checked.Add(1)
				if samePCM(lo, plain) && samePCM(hi, plain) {
					inaudible.Add(1)
					t.Errorf("%s %s/%s: inaudible — neither end changes a sample", id, g.ID, d.Dial.ID)
					continue
				}
				if d.Dial.Metric == "" {
					continue
				}
				mlo, ok1 := measure(id, d.Dial.Metric, lo)
				mmid, ok0 := measure(id, d.Dial.Metric, plain)
				mhi, ok2 := measure(id, d.Dial.Metric, hi)
				if !ok1 || !ok2 || !ok0 {
					continue // the descriptor does not apply to this sound
				}
				// Each end against the shipped sound, only when ≥ 10 away. A metric flat across a side is saturated or below its
				// frame resolution; the dial's audibility is TestEveryDialIsMeasurable's.
				tolOf := func(ref float64) float64 {
					if tol := 0.05 * math.Abs(ref); tol >= 0.021 {
						return tol
					}
					return 0.021 // two analysis frames on the time descriptors
				}
				sign := 1.0
				if d.Dial.MetricFalls {
					sign = -1
				}
				want := map[bool]string{false: "rise", true: "fall"}[d.Dial.MetricFalls]
				key := id + " " + g.ID + "/" + d.Dial.ID
				if 100-r0 >= dialMeasurableMinGap {
					if wrong := sign*(mhi-mmid) < -tolOf(mmid); wrong != (dialMetricExceptions[key+" up"] != "") {
						if wrong {
							t.Errorf("%s %s/%s: turning UP (%d → 100) moved %s %.4g → %.4g, want it to %s",
								id, g.ID, d.Dial.ID, r0, d.Dial.Metric, mmid, mhi, want)
						} else {
							t.Errorf("%s up is listed in dialMetricExceptions but moves its metric the right way now — remove it", key)
						}
					}
				}
				if r0 >= dialMeasurableMinGap {
					if wrong := sign*(mmid-mlo) < -tolOf(mmid); wrong != (dialMetricExceptions[key+" down"] != "") {
						if wrong {
							t.Errorf("%s %s/%s: turning DOWN (%d → 0) moved %s %.4g → %.4g, want the opposite of %s",
								id, g.ID, d.Dial.ID, r0, d.Dial.Metric, mmid, mlo, want)
						} else {
							t.Errorf("%s down is listed in dialMetricExceptions but moves its metric the right way now — remove it", key)
						}
					}
				}
			}
		}
	}
}

// A ratchet: a new wrong-way side fails, and a listed side that now moves the right way must go.
var dialMetricExceptions = map[string]string{
	// Empty: on the played voice every declared metric moves the declared way.
}

func samePCM(a, b []float32) bool {
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
