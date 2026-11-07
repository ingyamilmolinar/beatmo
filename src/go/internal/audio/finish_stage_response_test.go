//go:build !test && !js

package audio

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// Each op is measured as what it IS (filter, shaper, resampler) against known signals through the stage alone: a voice
// and the normaliser after it hide what an op does per frequency, which is how Crisp shipped as a broadband cut.

// The device rates and the 22.05 kHz analysis renders.
var finishSampleRates = []int{22050, 44100, 48000}

func finishBlock(over map[string]float64) modularBlock {
	b := zeroModularBlock()
	for k, v := range modularFieldInits() {
		if _, ok := modularSchemaIndex[k]; ok {
			b.set(k, v)
		}
	}
	for k, v := range over {
		b.set(k, v)
	}
	return b
}

func finishSine(sr, n int, hz, amp float64) []float32 {
	buf := make([]float32, n)
	for i := range buf {
		buf[i] = float32(amp * math.Sin(2*math.Pi*hz*float64(i)/float64(sr)))
	}
	return buf
}

func finishApply(sr int, in []float32, over map[string]float64) []float32 {
	out := append([]float32(nil), in...)
	applyModularFinish(out, sr, finishBlock(over))
	return out
}

// Over the second half of one second, once the filters have settled.
func finishSineGainDB(sr int, hz float64, over map[string]float64) float64 {
	in := finishSine(sr, sr, hz, 0.25)
	out := finishApply(sr, in, over)
	var a, b float64
	for i := sr / 2; i < sr; i++ {
		a += float64(in[i]) * float64(in[i])
		b += float64(out[i]) * float64(out[i])
	}
	return 10 * math.Log10(b/a)
}

// Hann-windowed so a neighbouring partial a few hertz away cannot leak in.
func toneAmplitude(buf []float32, sr int, hz float64, from, to int) float64 {
	var re, im, wsum float64
	n := to - from
	for i := from; i < to; i++ {
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i-from)/float64(n-1))
		ph := 2 * math.Pi * hz * float64(i) / float64(sr)
		re += w * float64(buf[i]) * math.Cos(ph)
		im += w * float64(buf[i]) * math.Sin(ph)
		wsum += w
	}
	return 2 * math.Hypot(re, im) / wsum
}

func rmsOf(buf []float32) float64 {
	var s float64
	for _, v := range buf {
		s += float64(v) * float64(v)
	}
	return math.Sqrt(s / float64(len(buf)))
}

// A voice that leaves FINISH alone renders exactly as without it.
func TestFinishIdentityIsBitExact(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	in := make([]float32, 48000)
	for i := range in {
		in[i] = float32(rng.Float64()*2 - 1)
	}
	for _, sr := range finishSampleRates {
		out := finishApply(sr, in, nil)
		for i := range in {
			if out[i] != in[i] {
				t.Fatalf("sr %d: identity FINISH changed sample %d: %v → %v", sr, i, in[i], out[i])
			}
		}
	}
}

// A HIGH SHELF: the body keeps its level, the top rises in proportion, the same at every rate.
func TestFinishCrispLiftsTheHighsAndLeavesTheLowsAlone(t *testing.T) {
	for _, sr := range finishSampleRates {
		prevHigh := 0.0
		for _, a := range []float64{0.25, 0.5, 1} {
			over := map[string]float64{"post_brightness": a}
			for _, hz := range []float64{60, 200, 400} {
				if g := finishSineGainDB(sr, hz, over); math.Abs(g) > 0.5 {
					t.Errorf("sr %d Crisp %.2f: %.0f Hz moved %+.2f dB — the lows must keep their level", sr, a, hz, g)
				}
			}
			high := finishSineGainDB(sr, 9000, over)
			if want := 9 * a; high < want {
				t.Errorf("sr %d Crisp %.2f: 9 kHz %+.2f dB, want ≥ %+.1f dB", sr, a, high, want)
			}
			if high > 12.5 {
				t.Errorf("sr %d Crisp %.2f: 9 kHz %+.2f dB, want ≤ +12.5 dB", sr, a, high)
			}
			if high <= prevHigh {
				t.Errorf("sr %d Crisp %.2f: 9 kHz %+.2f dB does not rise above %+.2f dB at the previous step", sr, a, high, prevHigh)
			}
			prevHigh = high
			if mid := finishSineGainDB(sr, 3000, over); mid <= 0.5 || mid >= high {
				t.Errorf("sr %d Crisp %.2f: 3 kHz %+.2f dB, want a lift between the lows (0) and the top (%+.2f)", sr, a, mid, high)
			}
		}
	}
}

// A LOW SHELF: the bottom rises, mids and highs keep their level.
func TestFinishLowBoostLiftsTheLowsAndLeavesTheRestAlone(t *testing.T) {
	for _, sr := range finishSampleRates {
		prevLow := 0.0
		for _, a := range []float64{0.25, 0.5, 1} {
			over := map[string]float64{"post_body": a}
			for _, hz := range []float64{1500, 4000, 9000} {
				if g := finishSineGainDB(sr, hz, over); math.Abs(g) > 0.5 {
					t.Errorf("sr %d Low Boost %.2f: %.0f Hz moved %+.2f dB — the mids and highs must keep their level", sr, a, hz, g)
				}
			}
			low := finishSineGainDB(sr, 40, over)
			if want := 9 * a; low < want {
				t.Errorf("sr %d Low Boost %.2f: 40 Hz %+.2f dB, want ≥ %+.1f dB", sr, a, low, want)
			}
			if low > 12.5 {
				t.Errorf("sr %d Low Boost %.2f: 40 Hz %+.2f dB, want ≤ +12.5 dB", sr, a, low)
			}
			if low <= prevLow {
				t.Errorf("sr %d Low Boost %.2f: 40 Hz %+.2f dB does not rise above %+.2f dB at the previous step", sr, a, low, prevLow)
			}
			prevLow = low
		}
	}
}

// A low-pass crossfaded by its amount.
func TestFinishDarkenCutsTheHighsAndLeavesTheLowsAlone(t *testing.T) {
	for _, sr := range finishSampleRates {
		prevHigh := 0.0
		for _, a := range []float64{0.25, 0.5, 1} {
			over := map[string]float64{"post_darken": a}
			for _, hz := range []float64{60, 200} {
				if g := finishSineGainDB(sr, hz, over); math.Abs(g) > 0.5 {
					t.Errorf("sr %d Darken %.2f: %.0f Hz moved %+.2f dB — the lows must keep their level", sr, a, hz, g)
				}
			}
			high := finishSineGainDB(sr, 9000, over)
			if high >= prevHigh {
				t.Errorf("sr %d Darken %.2f: 9 kHz %+.2f dB does not fall below %+.2f dB at the previous step", sr, a, high, prevHigh)
			}
			prevHigh = high
		}
		if prevHigh > -9 {
			t.Errorf("sr %d Darken 1: 9 kHz only %+.2f dB, want a cut of at least 9 dB", sr, prevHigh)
		}
	}
}

// Odd harmonics only (tanh is odd), growing with the knob.
func TestFinishCrunchAddsHarmonicsInProportion(t *testing.T) {
	const sr, hz = 48000, 200.0
	in := finishSine(sr, sr, hz, 0.5)
	prev := 0.0
	for _, a := range []float64{0.1, 0.25, 0.5, 1} {
		out := finishApply(sr, in, map[string]float64{"post_drive": a})
		fund := toneAmplitude(out, sr, hz, 0, sr)
		third := toneAmplitude(out, sr, 3*hz, 0, sr) / fund
		second := toneAmplitude(out, sr, 2*hz, 0, sr) / fund
		if third <= prev {
			t.Errorf("Crunch %.2f: 3rd harmonic %.4f of the fundamental does not rise above %.4f", a, third, prev)
		}
		prev = third
		if second > 1e-4 {
			t.Errorf("Crunch %.2f: an EVEN harmonic appeared (%.2g of the fundamental) — the shaper is not odd-symmetric", a, second)
		}
		if peak, _ := finitePeak(out); peak > 1.01 {
			t.Errorf("Crunch %.2f: peak %.3f — the shaper must stay within full scale for an in-range input", a, peak)
		}
	}
	if prev < 0.05 {
		t.Errorf("Crunch 1: 3rd harmonic only %.3f of the fundamental — barely saturating", prev)
	}
}

// At 2^(st/12) exactly, at the input level; a shift up leaves silence after the shortened voice.
func TestFinishResampleShiftsPitchExactly(t *testing.T) {
	for _, sr := range finishSampleRates {
		for _, st := range []float64{-12, -7, -1, 3, 12, 19} {
			const hz = 330.0
			n := sr
			in := finishSine(sr, n, hz, 0.5)
			out := finishApply(sr, in, map[string]float64{"post_pitch": st})
			rate := math.Pow(2, st/12)
			want := hz * rate
			end := n
			if rate > 1 {
				end = int(float64(n)/rate) - 64
			}
			from := end / 4
			got := toneAmplitude(out, sr, want, from, end)
			if math.Abs(got-0.5) > 0.5*0.03 {
				t.Errorf("sr %d Resample %+g st: %.1f Hz at amplitude %.4f, want 0.5 (±3%%)", sr, st, want, got)
			}
			if stray := toneAmplitude(out, sr, hz, from, end); st != 0 && stray > 0.01 {
				t.Errorf("sr %d Resample %+g st: the ORIGINAL %.0f Hz is still there at %.4f", sr, st, hz, stray)
			}
			if rate > 1 {
				for i := int(float64(n)/rate) + 64; i < n; i++ {
					if out[i] != 0 {
						t.Errorf("sr %d Resample %+g st: sample %d after the shortened voice is %v, want silence", sr, st, i, out[i])
						break
					}
				}
			}
		}
	}
}

// Content pushed past Nyquist is removed, not folded back as inharmonic tones.
func TestFinishResampleDoesNotAlias(t *testing.T) {
	for _, sr := range finishSampleRates {
		nyq := float64(sr) / 2
		for _, st := range []float64{7, 12, 24} {
			rate := math.Pow(2, st/12)
			// A partial that lands at 1.4× Nyquist after the shift.
			hz := 1.4 * nyq / rate
			n := sr
			in := finishSine(sr, n, hz, 0.5)
			out := finishApply(sr, in, map[string]float64{"post_pitch": st})
			end := int(float64(n)/rate) - 64
			alias := 2*nyq - hz*rate // where linear interpolation folds it
			if lvl := toneAmplitude(out, sr, alias, end/4, end); lvl > 0.5*0.01 {
				t.Errorf("sr %d Resample %+g st: a %.0f Hz partial (→ %.0f Hz, past Nyquist) folds back to %.0f Hz at %.1f dB, want ≤ −40 dB",
					sr, st, hz, hz*rate, alias, 20*math.Log10(lvl/0.5))
			}
			if r := rmsOf(out[end/4 : end]); r > 0.5/math.Sqrt2*0.01 {
				t.Errorf("sr %d Resample %+g st: %.1f dB of the out-of-band partial survives in some form, want ≤ −40 dB",
					sr, st, 20*math.Log10(r/(0.5/math.Sqrt2)))
			}
		}
	}
}

// Content still below Nyquist keeps its level.
func TestFinishResampleKeepsThePassbandFlat(t *testing.T) {
	for _, sr := range finishSampleRates {
		nyq := float64(sr) / 2
		for _, st := range []float64{-12, -1, 5, 12} {
			rate := math.Pow(2, st/12)
			for _, frac := range []float64{0.1, 0.4, 0.75} {
				hz := frac * nyq / math.Max(rate, 1)
				n := sr
				out := finishApply(sr, finishSine(sr, n, hz, 0.5), map[string]float64{"post_pitch": st})
				end := n
				if rate > 1 {
					end = int(float64(n)/rate) - 64
				}
				if got := toneAmplitude(out, sr, hz*rate, end/4, end); math.Abs(20*math.Log10(got/0.5)) > 0.5 {
					t.Errorf("sr %d Resample %+g st: %.0f Hz (→ %.0f Hz) came out at %+.2f dB, want within ±0.5 dB",
						sr, st, hz, hz*rate, 20*math.Log10(got/0.5))
				}
			}
		}
	}
}

// exp(−t·rate·(1/L − 1)) from full level, no step at the onset, finite at L = 0.
func TestFinishFadeIsAnExponentialAtItsRate(t *testing.T) {
	const sr = 48000
	in := make([]float32, sr)
	for i := range in {
		in[i] = 0.5
	}
	prevHalf := math.Inf(1)
	for _, c := range []struct{ length, rate float64 }{{0.9, 8}, {0.5, 8}, {0.5, 20}, {0.2, 8}, {0, 8}} {
		out := finishApply(sr, in, map[string]float64{"post_decay": c.length, "post_decay_rate": c.rate})
		L := math.Max(c.length, 0.01)
		k := c.rate * (1/L - 1)
		for _, i := range []int{0, 1, sr / 100, sr / 10, sr / 2} {
			want := 0.5 * math.Exp(-float64(i)/sr*k)
			if math.Abs(float64(out[i])-want) > 1e-6+1e-5*want {
				t.Errorf("Length %.2f rate %.0f: sample %d = %.7g, want %.7g", c.length, c.rate, i, out[i], want)
			}
		}
		if c.rate != 8 {
			continue
		}
		half := math.Ln2 / k
		if half >= prevHalf {
			t.Errorf("Length %.2f: half-life %.4fs is not shorter than at the previous (longer) Length", c.length, half)
		}
		prevHalf = half
	}
}

// 0 means no ceiling.
func TestFinishLimitClampsPeaksAtTheCeiling(t *testing.T) {
	const sr = 48000
	in := finishSine(sr, sr, 440, 1.2)
	for _, c := range []float64{0.3, 1, 1.5} {
		out := finishApply(sr, in, map[string]float64{"out_clamp": c})
		for i := range in {
			want := math.Max(-c, math.Min(c, float64(in[i])))
			if math.Abs(float64(out[i])-want) > 1e-6 {
				t.Fatalf("Limit %.2f: sample %d = %v from %v, want %v", c, i, out[i], in[i], want)
			}
		}
	}
	out := finishApply(sr, in, map[string]float64{"out_clamp": 0})
	for i := range in {
		if out[i] != in[i] {
			t.Fatalf("Limit 0 (no ceiling): sample %d changed %v → %v", i, in[i], out[i])
		}
	}
}

// A NaN/Inf reaching the stage leaves as silence; every op at every extreme keeps finite audio finite.
func TestFinishStageNeverEmitsNonFinite(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	in := make([]float32, 24000)
	for i := range in {
		in[i] = float32(rng.Float64()*2 - 1)
	}
	bad := append([]float32(nil), in...)
	bad[100], bad[200] = float32(math.NaN()), float32(math.Inf(1))
	extremes := []map[string]float64{
		{"post_pitch": -24}, {"post_pitch": 24}, {"post_decay": 0}, {"post_decay_rate": 64, "post_decay": 0.01},
		{"post_body": 1}, {"post_brightness": 1}, {"post_darken": 1}, {"post_drive": 1},
		{"post_pitch": 24, "post_body": 1, "post_brightness": 1, "post_drive": 1},
	}
	for _, sr := range finishSampleRates {
		for _, over := range extremes {
			name := fmt.Sprint(over)
			out := finishApply(sr, in, over)
			if peak, nf := finitePeak(out); nf > 0 || peak > 8 {
				t.Errorf("sr %d %s: %d non-finite samples, peak %.3g from full-scale noise", sr, name, nf, peak)
			}
			poisoned := finishApply(sr, bad, over)
			if _, nf := finitePeak(poisoned); nf > 0 {
				t.Errorf("sr %d %s: a NaN/Inf input leaves as %d non-finite samples", sr, name, nf)
			}
			// One bad sample costs that sample: a recursive filter fed a NaN holds it forever, and the guard silences the rest.
			ref := rmsOf(out[3000:5500])
			if got := rmsOf(poisoned[3000:5500]); got < 0.5*ref {
				t.Errorf("sr %d %s: after a NaN at sample 100 the voice drops to %.3g RMS (clean %.3g) — the bad sample poisoned the filter state",
					sr, name, got, ref)
			}
		}
	}
}
