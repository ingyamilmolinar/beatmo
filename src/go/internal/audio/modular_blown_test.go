//go:build !test && !js

package audio

import (
	"fmt"
	"math"
	"testing"
)

// Karplus-Strong in BLOWN mode at an absolute frequency, everything else silenced.
func blownKSParams(freq, blow, sustain, pluck float64) modularBlock {
	p := defaultModularParams()
	p.set("osc_enabled", 0)
	for i := range modularGenSlots {
		p.set(fmt.Sprintf("gen%d_source", i+1), 0)
		p.set(fmt.Sprintf("gen%d_gain", i+1), 0)
	}
	p.set("gen1_source", 3)    // Karplus-Strong
	p.set("gen1_freq_mode", 1) // absolute Hz (no voice_freq dependency)
	p.set("gen1_freq", freq)
	p.set("gen1_gain", 1.0)
	p.set("gen1_ks_blow", blow)
	p.set("gen1_ks_sustain", sustain)
	p.set("gen1_ks_pluck", pluck)
	p.set("gen1_out_scale", 0.85) // KS output scale (recipe supplies this; 0 would silence the slot)
	p.set("gen1_env_fast_rate", 1.8)
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

// 1.0 = periodic at that lag, ~0 = noise.
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

// The jet's cubic must bound the oscillation at every feedback gain and pitch, below the hard-clamp ceiling.
func TestBlownKS_BoundedAcrossFeedbackAndPitch(t *testing.T) {
	const sr = 48000
	n := sr // 1s — long enough for any runaway buildup to show
	// Feedback ≥ 1: the saturator must bound the limit cycle where the linear loop gain exceeds unity.
	for _, freq := range []float64{220, 440, 660, 1047} {
		for _, fb := range []float64{0.90, 0.99, 0.999, 1.0, 1.05} {
			buf := make([]float32, n)
			renderModularBlock(buf, sr, n, blownKSParams(freq, 0.5, fb, 0.85))
			assertFinite(t, buf)
			if pk := peakAbs(buf); pk > 1.5 {
				t.Fatalf("blown KS freq=%.0f fb=%.3f UNBOUNDED (peak=%.3f) — jet failed to self-limit", freq, fb, pk)
			}
		}
	}
}

// A blown voice still oscillates in the tail where a pluck has decayed.
func TestBlownKS_SustainsVsPluckDecays(t *testing.T) {
	const sr = 48000
	n := sr
	// High feedback (≈1) = a sustained blown tube; the tanh bounds the buildup.
	blown := make([]float32, n)
	renderModularBlock(blown, sr, n, blownKSParams(660, 0.35, 0.999, 0.5))
	pl := make([]float32, n)
	renderModularBlock(pl, sr, n, blownKSParams(660, 0 /*pluck*/, 0.999, 0.5))

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

// Resonates at the note (autocorrelation peaks at the period), not broadband noise.
func TestBlownKS_Pitched(t *testing.T) {
	const sr = 48000
	n := sr
	freq := 660.0
	buf := make([]float32, n)
	renderModularBlock(buf, sr, n, blownKSParams(freq, 0.3, 0.99, 0.5))
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
