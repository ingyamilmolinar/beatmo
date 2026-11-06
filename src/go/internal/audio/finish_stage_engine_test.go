//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// finish_stage_engine_test.go — the FINISH stage (the always-on post ops) run
// through the real C engine at each instrument's real render length. The C0
// parity cases render 250 ms with one knob moved at a time, which is how a Fade
// that GREW the envelope (exp of a positive exponent → Inf/NaN after a second)
// shipped as a visible knob.

var finishEngineInstruments = []instrument.ID{roster.Kick, roster.Snare, roster.Violin, roster.FmBell}

func finishDefs(t *testing.T, id string) []ParamDef {
	t.Helper()
	defs := InstrumentParamDefs(instrument.ID(id))
	if len(defs) == 0 {
		t.Fatalf("%s: no instrument document", id)
	}
	var out []ParamDef
	for _, d := range defs {
		if d.Group == FinishGroup {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s: no FINISH params", id)
	}
	return out
}

// renderFinish renders id at its document's render length with overrides set
// through its document.
func renderFinish(t *testing.T, id instrument.ID, over map[string]float64) []float32 {
	t.Helper()
	sec := 1.0
	if d, ok := ShippedDoc(id); ok && d.Render.Seconds > 0 {
		sec = d.Render.Seconds
	}
	sr := SampleRate()
	return renderFieldsForTest(t, id, sr, int(sec*float64(sr)), over)
}

func finitePeak(buf []float32) (peak float64, nonFinite int) {
	for _, s := range buf {
		v := float64(s)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			nonFinite++
			continue
		}
		peak = math.Max(peak, math.Abs(v))
	}
	return peak, nonFinite
}

// TestFinishKnobExtremesStayFiniteAndBounded: every pair of FINISH knobs at
// every combination of their extremes renders finite audio no louder than a
// few times the untouched voice. The post ops shape a finished voice — none of
// them may turn it into a runaway (Inf poisons the browser's peak-normaliser
// and then every BiquadFilter on the channel).
func TestFinishKnobExtremesStayFiniteAndBounded(t *testing.T) {
	for _, id := range finishEngineInstruments {
		base, _ := finitePeak(renderFinish(t, id, nil))
		if base == 0 {
			t.Fatalf("%s: default render is silent", id)
		}
		defs := finishDefs(t, string(id))
		for i := range defs {
			for j := i + 1; j < len(defs); j++ {
				a, b := defs[i], defs[j]
				for _, va := range []float64{a.Min, a.Max} {
					for _, vb := range []float64{b.Min, b.Max} {
						peak, bad := finitePeak(renderFinish(t, id, map[string]float64{a.Name: va, b.Name: vb}))
						if bad > 0 || peak > 8*base {
							t.Errorf("%s %s=%v %s=%v: %d non-finite samples, peak %.3g (default %.3g)",
								id, a.Name, va, b.Name, vb, bad, peak, base)
						}
					}
				}
			}
		}
	}
}

// TestFinishKnobsRespondProportionally: the first 1% of a FINISH knob's travel
// does a small part of its work, not most of it. Darken used to switch in a
// full 8 kHz low-pass at 0.01 and Crunch a full tanh(x)/tanh(1) at 0.01 — the
// knob read as a switch with a decorative tail.
func TestFinishKnobsRespondProportionally(t *testing.T) {
	dist := func(a, b []float32) float64 {
		var s float64
		for i := range a {
			d := float64(a[i]) - float64(b[i])
			s += d * d
		}
		return math.Sqrt(s)
	}
	ident := modularFieldInits()
	for _, id := range finishEngineInstruments {
		dry := renderFinish(t, id, nil)
		for _, d := range finishDefs(t, string(id)) {
			switch d.Name {
			case "post_pitch", "out_clamp", "post_decay_rate":
				continue // a resample, a ceiling sentinel and a rate: not amounts
			}
			id0 := ident[d.Name]
			far := d.Max
			if math.Abs(d.Min-id0) > math.Abs(d.Max-id0) {
				far = d.Min
			}
			step := id0 + 0.01*(far-id0)
			full := dist(renderFinish(t, id, map[string]float64{d.Name: far}), dry)
			small := dist(renderFinish(t, id, map[string]float64{d.Name: step}), dry)
			if full == 0 {
				t.Errorf("%s %s: the knob's full travel changes nothing", id, d.Name)
				continue
			}
			if small > 0.1*full {
				t.Errorf("%s %s: 1%% of the travel (%.4g) already does %.0f%% of the full change",
					id, d.Name, step, 100*small/full)
			}
		}
	}
}
