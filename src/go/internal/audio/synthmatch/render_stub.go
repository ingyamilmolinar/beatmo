//go:build test

package synthmatch

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// RenderInstrument renders instID at the given pitch (semitones from A3=220Hz)
// for durSec (0 = instrument default) at sr, returning a mono wave.Wave (float64).
// Test build: uses a pure-Go param-responsive synth (no CGo).
func RenderInstrument(instID string, pitch float64, sr int, durSec float64) (wave.Wave, error) {
	return RenderWithParams(instID, pitch, sr, durSec, nil)
}

// RenderWithParams is RenderInstrument with extra param overrides applied on top
// of the merged recipe defaults (and "pitch") before rendering.
//
// Test build: a small deterministic pure-Go synth that responds to the tunable
// params so the optimizer can descend under -tags test. It does not need to
// sound real — it provides smooth, monotonic-ish sensitivity to the params the
// TunableSpecs expose. No RNG — fully deterministic given the same inputs.
func RenderWithParams(instID string, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error) {
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
	return wave.Wave{Samples: out, SampleRate: sr, Label: instID}, nil
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

