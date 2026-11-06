//go:build test

package synthmatch

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// RenderInstrument renders instID at the given pitch (semitones from A3=220Hz)
// for durSec (0 = instrument default) at sr, returning a mono wave.Wave (float64).
// Test build: uses a pure-Go param-responsive synth (no CGo).
func RenderInstrument(instID instrument.ID, pitch float64, sr int, durSec float64) (wave.Wave, error) {
	return RenderWithParams(instID, pitch, sr, durSec, nil)
}

// RenderWithParams is RenderInstrument with extra param overrides set through
// the instrument's document (and "pitch") before rendering.
//
// Test build: a small deterministic pure-Go synth that responds to the tunable
// params so the optimizer can descend under -tags test. It does not need to
// sound real — it provides smooth, monotonic-ish sensitivity to the params the
// TunableSpecs expose. No RNG — fully deterministic given the same inputs.
func RenderWithParams(instID instrument.ID, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error) {
	_, merged, samples, err := resolveRender(instID, pitch, sr, durSec, overrides)
	if err != nil {
		return wave.Wave{}, err
	}
	get := func(key string, def float64) float64 {
		if v, ok := merged[key]; ok {
			return v
		}
		return def
	}
	freq := 220.0 * math.Pow(2, pitch/12.0)
	cutoff := get("filter_cutoff", 4000)
	reso := get("filter_resonance", 1.0)
	atk := get("amp_attack", 0.01)
	dec := get("amp_decay", 0.1)
	sus := get("amp_sustain", 0.8)
	rel := get("amp_release", 0.1)
	lfoRate := get("lfo_rate", 0)
	lfoDepth := get("lfo_depth", 0)
	ks := get("gen1_ks_sustain", 0) // 0 if not a KS instrument

	// Per-harmonic base amplitudes, scaled by gen{k}_gain if present.
	const H = 8
	var amp [H + 1]float64
	for k := 1; k <= H; k++ {
		a := 1.0 / float64(k)
		if g, ok := merged[genGainKey(k)]; ok {
			a *= g
		}
		// One-pole-ish lowpass by cutoff; resonance bumps the harmonic nearest cutoff.
		hz := freq * float64(k)
		a /= 1 + (hz/cutoff)*(hz/cutoff)
		if math.Abs(hz-cutoff) < freq/2 {
			a *= 1 + 0.5*reso
		}
		amp[k] = a
	}

	dur := float64(samples) / float64(sr)
	out := make([]float64, samples)
	for i := range out {
		t := float64(i) / float64(sr)
		env := adsrEnv(t, dur, atk, dec, sus, rel)
		if ks > 0 { // KS "sustain" lengthens ring: bias the envelope toward sustain
			env = env*(1-ks) + ks*math.Exp(-t*(2.0-ks))
		}
		f := freq
		if lfoDepth > 0 && lfoRate > 0 {
			f *= 1 + lfoDepth*math.Sin(2*math.Pi*lfoRate*t)
		}
		s := 0.0
		for k := 1; k <= H; k++ {
			s += amp[k] * math.Sin(2*math.Pi*f*float64(k)*t)
		}
		out[i] = 0.5 * env * s
	}
	return wave.Wave{Samples: out, SampleRate: sr, Label: string(instID)}, nil
}

// genGainKey returns the param key for the k-th generator gain: "gen1_gain".."gen8_gain".
func genGainKey(k int) string {
	return "gen" + string(rune('0'+k)) + "_gain"
}

// adsrEnv returns the ADSR envelope value at time t over a note of length dur.
func adsrEnv(t, dur, a, d, s, r float64) float64 {
	if a < 1e-4 {
		a = 1e-4
	}
	relStart := dur - r
	if relStart < a {
		relStart = a
	}
	switch {
	case t < a:
		return t / a
	case t < a+d:
		x := (t - a) / math.Max(d, 1e-4)
		return 1 + (s-1)*x
	case t < relStart:
		return s
	case t < dur:
		x := (t - relStart) / math.Max(r, 1e-4)
		return s * (1 - x)
	default:
		return 0
	}
}

// RenderShipped is the test-build stand-in: the pure-Go synth of
// RenderWithParams (there is no C engine, FX or normalisation here).
func RenderShipped(instID instrument.ID, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error) {
	return RenderWithParams(instID, pitch, sr, durSec, overrides)
}

// RenderHeld is the test-build stand-in: the pure-Go synth's voice is
// durSec long (0 = the instrument default) whatever it is held for.
func RenderHeld(instID instrument.ID, pitch float64, sr int, durSec, voiceSec float64, overrides map[string]float64) (wave.Wave, error) {
	return RenderWithParams(instID, pitch, sr, durSec, overrides)
}

// HeldSeconds: the stub synth ignores how long a voice is held (0 = the
// shipped length).
func HeldSeconds(instID instrument.ID, pitch, minSec float64) float64 { return 0 }

// RenderShippedDoc is the test-build stand-in for a document candidate: a
// decaying sine at the pitch, its decay from the document's render length —
// the session plumbing runs, no C engine.
func RenderShippedDoc(d instrument.Doc, pitch float64, sr int, durSec float64) (wave.Wave, error) {
	if durSec <= 0 {
		durSec = d.Render.Seconds
	}
	n := max(1, int(float64(sr)*durSec))
	f := 220 * math.Pow(2, pitch/12)
	out := make([]float64, n)
	for i := range out {
		t := float64(i) / float64(sr)
		out[i] = math.Exp(-3*t/math.Max(d.Render.Seconds, 0.1)) * math.Sin(2*math.Pi*f*t)
	}
	return wave.Wave{Samples: out, SampleRate: sr, Label: string(d.ID)}, nil
}

// HeldSecondsDoc mirrors HeldSeconds for an unregistered document.
func HeldSecondsDoc(d instrument.Doc, pitch, minSec float64) float64 {
	return math.Max(d.Render.Seconds, minSec)
}

// RenderHeldDoc is the test-build stand-in for an unregistered document held
// voiceSec long: a small deterministic synth that RESPONDS to the
// document -- each ON gen.wavetable layer is a partial at its ratio and gain,
// each ON gen.noise layer adds that much noise, an ON filter stage low-passes
// at filter_cutoff -- so code that searches a document's values (refs tune)
// has a landscape to descend under -tags test. It does not sound real.
func RenderHeldDoc(d instrument.Doc, pitch float64, sr int, durSec, voiceSec float64) (wave.Wave, error) {
	if durSec <= 0 {
		durSec = math.Max(d.Render.Seconds, 0.25)
	}
	n := max(1, int(float64(sr)*durSec))
	f0 := 220 * math.Pow(2, pitch/12)
	out := make([]float64, n)
	seed := uint32(12345)
	for _, l := range d.Layers {
		if l == nil || !l.On {
			continue
		}
		g := l.Values["gain"]
		switch l.Type {
		case "gen.wavetable":
			f := f0 * l.Values["freq"]
			for i := range out {
				out[i] += g * math.Sin(2*math.Pi*f*float64(i)/float64(sr))
			}
		case "gen.noise":
			for i := range out {
				seed = seed*1664525 + 1013904223
				out[i] += g * (float64(seed)/float64(1<<32)*2 - 1)
			}
		}
	}
	if st, ok := d.Stages["filter"]; ok && st.On {
		if fc := st.Values["filter_cutoff"]; fc > 0 {
			a := 1 - math.Exp(-2*math.Pi*fc/float64(sr))
			y := 0.0
			for i, x := range out {
				y += a * (x - y)
				out[i] = y
			}
		}
	}
	for i := range out {
		out[i] *= math.Exp(-2 * float64(i) / float64(n))
	}
	return wave.Wave{Samples: out, SampleRate: sr, Label: string(d.ID)}, nil
}
