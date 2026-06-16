//go:build !test && !js

package audio

import (
	"math"
	"math/cmplx"
	"testing"
)

// template_bass_spectral_test.go is the evidence-backed half of the "bass guitar
// sounds metallic" regression guard (the shipped templates avoid bass-guitar for
// bass roles; TestBassGuitarBannedFromTemplates pins why).
//
// The circuit templates drive bass lines through per-node pitch, which the
// engine realises via naive linear-interpolation resampling (PlayParams). An
// instrument whose render is already harmonically bright reads as a metallic
// pluck — not a bass — and resampling only makes it worse. This test renders
// each candidate bass instrument and measures the fraction of spectral energy
// above 2 kHz: a real bass note (≤110 Hz fundamental) should sit almost entirely
// below ~1 kHz. fm-bass / sub-bass must stay clean; bass-guitar is pinned as
// intrinsically bright so the contrast (and the reason it is banned from bass
// roles) is documented, not folklore.

func bassHFEnergyFraction(render func(buf []float32, sampleRate, samples int)) float64 {
	const sr = 48000
	const n = sr // 1.0s
	fb := make([]float32, n)
	render(fb, sr, n)

	const N = 2048
	start := sr / 20 // skip the 50 ms attack transient
	if start+N > n {
		start = 0
	}
	seg := make([]complex128, N)
	for i := range N {
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(N-1))
		seg[i] = complex(float64(fb[start+i])*w, 0)
	}
	spec := naiveDFT(seg)
	binHz := float64(sr) / float64(N)
	cutBin := int(math.Floor(2000.0 / binHz))
	var total, hf float64
	for k := 1; k < N/2; k++ {
		p := cmplx.Abs(spec[k])
		p *= p
		total += p
		if k >= cutBin {
			hf += p
		}
	}
	if total == 0 {
		return 0
	}
	return hf / total
}

func naiveDFT(x []complex128) []complex128 {
	n := len(x)
	out := make([]complex128, n)
	for k := range n {
		var s complex128
		for t := range n {
			ang := -2 * math.Pi * float64(k) * float64(t) / float64(n)
			s += x[t] * cmplx.Exp(complex(0, ang))
		}
		out[k] = s
	}
	return out
}

func TestTemplateBassInstrumentsAreSpectrallyClean(t *testing.T) {
	// Clean basses templates are allowed to use: nearly all energy below 2 kHz.
	clean := []struct {
		name   string
		render func(buf []float32, sampleRate, samples int)
	}{
		{"fm-bass", renderFMBassVoice},
		{"sub-bass", renderSubBassVoice},
	}
	const cleanMax = 0.10 // ≤10% of energy above 2 kHz
	for _, c := range clean {
		hf := bassHFEnergyFraction(c.render)
		t.Logf("%-10s HF>2kHz = %.1f%%", c.name, hf*100)
		if hf > cleanMax {
			t.Errorf("%s: %.1f%% of energy above 2 kHz (>%.0f%%) — no longer a clean bass; "+
				"templates rely on it sounding deep", c.name, hf*100, cleanMax*100)
		}
	}

	// bass-guitar is intrinsically bright (Karplus-Strong, golden-locked). Pin
	// that fact: if it ever becomes clean, revisit cleanBassInstruments.
	hf := bassHFEnergyFraction(renderBassGuitarVoice)
	t.Logf("%-10s HF>2kHz = %.1f%% (intentionally banned from bass roles)", "bass-guitar", hf*100)
	if hf < 0.40 {
		t.Errorf("bass-guitar HF=%.1f%% is now low — it may be usable as a bass; "+
			"re-evaluate the cleanBassInstruments allowlist in internal/templates", hf*100)
	}
}
