//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Through the real C engine at each instrument's own length: short single-knob renders let a Fade that GREW the
// envelope (Inf/NaN after a second) ship as a visible knob.

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

// Every pair of FINISH extremes stays finite and within a few times the untouched voice: Inf poisons the browser's
// peak normaliser and every filter on the channel.
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

// The first 1 % of travel does a small part of the work, not most of it, or the knob reads as a switch.
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
