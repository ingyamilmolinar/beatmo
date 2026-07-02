//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// blownKSParams builds a minimal blown-flute voice: source==3 (Karplus-Strong)
// driven in BLOWN mode (gen_ks_blow>0) at an ABSOLUTE frequency, everything else
// silenced. Used to test the STK jet self-limiting in isolation.
func blownKSParams(freq, blow, sustain, pluck float64) ModularParams {
	p := defaultModularParams()
	p.OscEnabled = 0
	for i := range p.GenSource {
		p.GenSource[i] = 0
		p.GenGain[i] = 0
	}
	p.GenSource[0] = 3   // Karplus-Strong
	p.GenFreqMode[0] = 1 // absolute Hz (no voice_freq dependency)
	p.GenFreq[0] = freq
	p.GenGain[0] = 1.0
	p.GenKsBlow[0] = blow
	p.GenKsSustain[0] = sustain
	p.GenKsPluck[0] = pluck
	p.GenOutScale[0] = 0.85 // KS output scale (recipe supplies this; 0 would silence the slot)
	p.GenEnvFastRate[0] = 1.8
	return p
}

func rmsRange(buf []float32, lo, hi int) float64 {
	if lo < 0 {
		lo = 0
	}
	if hi > len(buf) {
		hi = len(buf)
	}
	if hi <= lo {
		return 0
	}
	var s float64
	for _, v := range buf[lo:hi] {
		s += float64(v) * float64(v)
	}
	return math.Sqrt(s / float64(hi-lo))
}

// autocorrNorm returns the normalized autocorrelation of seg at the given lag
// (1.0 = perfectly periodic at that lag, ~0 = uncorrelated/noise).
func autocorrNorm(seg []float32, lag int) float64 {
	if lag <= 0 || lag >= len(seg) {
		return 0
	}
	var s, e0 float64
	for i := 0; i+lag < len(seg); i++ {
		s += float64(seg[i]) * float64(seg[i+lag])
		e0 += float64(seg[i]) * float64(seg[i])
	}
	if e0 == 0 {
		return 0
	}
	return s / e0
}

// TestBlownKS_BoundedAcrossFeedbackAndPitch is the stability guard for the STK
// jet self-limiting (the cubic x^3-x must bound the oscillation at EVERY feedback
// gain and pitch). The earlier linear comb built up without bound — peak/ripple
// ran to clipping (30-120 dB). The jet must keep the output finite and below the
// hard-clamp ceiling at all settings.
func TestBlownKS_BoundedAcrossFeedbackAndPitch(t *testing.T) {
	const sr = 48000
	n := sr // 1s — long enough for any runaway buildup to show
	// Include feedback ≥1 (and >1) — the tanh saturator must bound the limit cycle
	// even when the linear loop gain exceeds unity (where the old comb ran away).
	for _, freq := range []float64{220, 440, 660, 1047} {
		for _, fb := range []float64{0.90, 0.99, 0.999, 1.0, 1.05} {
			buf := make([]float32, n)
			renderModularP(buf, sr, n, blownKSParams(freq, 0.5, fb, 0.85))
			assertFinite(t, buf)
			if pk := peakAbs(buf); pk > 1.5 {
				t.Fatalf("blown KS freq=%.0f fb=%.3f UNBOUNDED (peak=%.3f) — jet failed to self-limit", freq, fb, pk)
			}
		}
	}
}

// TestBlownKS_SustainsVsPluckDecays confirms the CONTINUOUS excitation: a blown
// voice still oscillates in the note tail, where a plucked KS (bBlow=0) has decayed.
func TestBlownKS_SustainsVsPluckDecays(t *testing.T) {
	const sr = 48000
	n := sr
	// High feedback (≈1) = a sustained blown tube; the tanh bounds the buildup.
	blown := make([]float32, n)
	renderModularP(blown, sr, n, blownKSParams(660, 0.35, 0.999, 0.5))
	pl := make([]float32, n)
	renderModularP(pl, sr, n, blownKSParams(660, 0 /*pluck*/, 0.999, 0.5))

	t.Logf("blown segment RMS: 1st=%.5f mid=%.5f tail=%.5f peak=%.4f", rmsRange(blown, 0, n/4), rmsRange(blown, n/3, 2*n/3), rmsRange(blown, 3*n/4, n), peakAbs(blown))
	blownTail := rmsRange(blown, 3*n/4, n)
	pluckTail := rmsRange(pl, 3*n/4, n)
	t.Logf("tail RMS: blown=%.5f plucked=%.5f", blownTail, pluckTail)
	if blownTail <= 1e-4 {
		t.Fatalf("blown KS tail is silent (rms=%.6f) — continuous excitation not sustaining", blownTail)
	}
	if blownTail <= pluckTail {
		t.Fatalf("blown tail rms %.5f should exceed plucked tail rms %.5f (continuous excitation)", blownTail, pluckTail)
	}
}

// TestBlownKS_Pitched confirms the blown tube RESONATES at the note frequency
// (autocorrelation peaks at the period lag) rather than producing broadband noise.
func TestBlownKS_Pitched(t *testing.T) {
	const sr = 48000
	n := sr
	freq := 660.0
	buf := make([]float32, n)
	renderModularP(buf, sr, n, blownKSParams(freq, 0.3, 0.99, 0.5))
	seg := buf[n/2:] // steady-state second half
	lag := int(float64(sr)/freq + 0.5)
	atPeriod := autocorrNorm(seg, lag)
	atHalf := autocorrNorm(seg, lag/2)
	t.Logf("autocorr @period(lag=%d)=%.3f  @half(lag=%d)=%.3f", lag, atPeriod, lag/2, atHalf)
	if atPeriod < 0.3 {
		t.Fatalf("blown KS at %.0f Hz not pitched: autocorr@period=%.3f (<0.3 → broadband noise)", freq, atPeriod)
	}
	if atPeriod <= atHalf {
		t.Fatalf("autocorr@period (%.3f) should exceed @half-period (%.3f) — no clear fundamental", atPeriod, atHalf)
	}
}
