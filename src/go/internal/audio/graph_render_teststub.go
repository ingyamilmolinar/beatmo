//go:build test

package audio

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// RenderDocForGraph under -tags test has no C engine: it renders a
// deterministic TOY voice from a handful of the document's values (amp
// envelope, pitch, a noise mix, a filter cutoff as a one-pole), so the UI's
// graph plumbing can be tested — that a turned dial re-renders, that a slot
// lands, that the picture moves. It is NOT the instrument's sound and is never
// linked into a real binary; the real contract is TestGraphRenderIsThePlayedVoice
// (real build).
func RenderDocForGraph(id instrument.ID, d instrument.Doc, pitch float64) ([]float32, int, error) {
	sr := SampleRate()
	if sr <= 0 {
		sr = 48000
	}
	v := FlatFieldMap(d)
	get := func(name string, def float64) float64 {
		if x, ok := v[name]; ok {
			return x
		}
		return def
	}
	n := int(GraphVoiceSec(d) * float64(sr))
	if n < 1 {
		n = 1
	}
	atk := math.Max(0.001, get("amp_attack", 0.005))
	dec := math.Max(0.005, get("amp_decay", 0.3))
	sus := get("amp_sustain", 0)
	hz := 220 * math.Pow(2, (get("pitch", 0)+pitch)/12)
	drop := get("pitchenv_amt", 0) + get("gen1_kick_pe_amt", 0)
	noise := math.Min(1, get("gen1_snare_noise_m", 0)/2)
	alpha := 1 - math.Exp(-2*math.Pi*math.Min(get("filter_cutoff", 20000), 20000)/float64(sr))
	out := make([]float32, n)
	seed := uint32(12345)
	phase, lp := 0.0, 0.0
	for i := range out {
		t := float64(i) / float64(sr)
		env := t / atk
		if t >= atk {
			env = sus + (1-sus)*math.Exp(-(t-atk)/dec)
		}
		f := hz * (1 + drop*math.Exp(-t/0.03))
		phase += 2 * math.Pi * f / float64(sr)
		seed = seed*1664525 + 1013904223
		nz := float64(seed)/float64(1<<31) - 1
		x := (1-noise)*math.Sin(phase) + noise*nz
		lp += alpha * (x - lp)
		out[i] = float32(0.8 * env * lp)
	}
	if e, ok := SampleEditFor(id); ok {
		out = ApplySampleEditToBuffer(out, sr, e)
	}
	return out, sr, nil
}

// GraphVoiceSec under -tags test: render.seconds, like every platform.
func GraphVoiceSec(d instrument.Doc) float64 { return d.Render.Seconds }
