package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// snare_metrics.go — snare-specific analysis. A snare = a TONAL DRUM BODY (the
// head/shell modes, a mid "thud" ~150-250 Hz) + the SNARE WIRES (a broadband
// buzz, the defining "snap"). The two have their OWN levels and decays — a "fat
// bottom" snare is body-heavy with a shorter bright buzz. The existing kick
// metrics give bands/flatness/T60/centroid but not the explicit TONE-vs-NOISE
// split or the body tuning, which are what you actually turn to tune a snare.

// SnareMetrics holds the tonal-body / snare-noise decomposition.
type SnareMetrics struct {
	BodyFundHz     float64 // dominant body partial (drum tuning)
	ToneNoiseRatio float64 // harmonic(tonal) energy / total, on the body window (1=pure tone, 0=pure noise)
	LowBodyFrac    float64 // 80-400 Hz energy fraction (the "fat bottom")
	HighBuzzFrac   float64 // 2k-10k Hz energy fraction (the wire "snap")
	BodyDecayMs    float64 // low-band (body) -20 dB decay time
	BuzzDecayMs    float64 // high-band (buzz) -20 dB decay time
	AttackMs       float64 // onset → envelope peak
	CentroidHz     float64
	Flatness       float64
}

const (
	snareFFT         = 8192
	snareBodySkipS   = 0.005 // start just after the stick transient
	snareBodyLenS    = 0.06  // early body window for the tone/noise split
	snareLowLo       = 80.0
	snareLowHi       = 400.0
	snareHighLo      = 2000.0
	snareHighHi      = 10000.0
	snareDecayDropDB = 20.0 // measure the -20 dB decay time (snares are short)
)

// AnalyzeSnare decomposes a snare one-shot into its tonal-body and snare-noise
// components and their decays.
func AnalyzeSnare(w wave.Wave) SnareMetrics {
	sr := w.SampleRate
	if sr <= 0 {
		sr = 44100
	}
	out := SnareMetrics{}
	if w.PeakSample() < 1e-9 {
		return out
	}
	w = PeakNormalize(w)
	x := w.Samples

	// --- spectral (tone/noise split, body tuning, brightness) on the body window
	a := secToSamples(snareBodySkipS, sr)
	b := a + secToSamples(snareBodyLenS, sr)
	if b > len(x) {
		b = len(x)
	}
	if a >= b {
		a, b = 0, len(x)
	}
	mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: x[a:b], SampleRate: sr}, snareFFT, wave.WindowHann)
	out.CentroidHz = spectralCentroidHz(mag, binHz)
	out.Flatness = spectralFlatnessOf(mag)

	total := 0.0
	low, high := 0.0, 0.0
	for i, m := range mag {
		f := float64(i) * binHz
		p := m * m
		total += p
		if f >= snareLowLo && f <= snareLowHi {
			low += p
		}
		if f >= snareHighLo && f <= snareHighHi {
			high += p
		}
	}
	if total > 0 {
		out.LowBodyFrac = low / total
		out.HighBuzzFrac = high / total
	}

	// Body fundamental: strongest peak in 100-350 Hz (the drum tuning).
	out.BodyFundHz = strongestPeakHz(mag, binHz, 100, 350)
	// Tone/noise ratio: harmonic-comb energy (±3% of n·BodyFund) vs total.
	if out.BodyFundHz > 0 {
		out.ToneNoiseRatio = harmonicFraction(mag, binHz, out.BodyFundHz, total)
	}

	// --- decays: low-band body vs high-band buzz envelopes.
	out.BodyDecayMs = bandDecayMs(x, sr, snareLowLo, snareLowHi)
	out.BuzzDecayMs = bandDecayMs(x, sr, snareHighLo, snareHighHi)
	out.AttackMs = snareAttackMs(x, sr)
	return out
}

// strongestPeakHz returns the frequency of the largest magnitude bin in [lo,hi].
func strongestPeakHz(mag []float64, binHz, lo, hi float64) float64 {
	best, bf := 0.0, 0.0
	for i, m := range mag {
		f := float64(i) * binHz
		if f < lo || f > hi {
			continue
		}
		if m > best {
			best, bf = m, f
		}
	}
	return bf
}

// harmonicFraction sums spectral power within ±3% of each harmonic n·f0 (the
// tonal body) and returns it as a fraction of the total power (residual = noise).
func harmonicFraction(mag []float64, binHz, f0, total float64) float64 {
	if total <= 0 || f0 <= 0 {
		return 0
	}
	harm := 0.0
	for n := 1; n <= 20; n++ {
		center := float64(n) * f0
		band := center*0.045 + 2*binHz // widen a touch + at least ±2 bins (short-window leakage)
		for i, m := range mag {
			f := float64(i) * binHz
			if math.Abs(f-center) <= band {
				harm += m * m
			}
		}
	}
	r := harm / total
	if r > 1 {
		r = 1
	}
	return r
}

// bandDecayMs returns the time (ms) for the band-limited RMS envelope to fall
// snareDecayDropDB below its peak (a robust short-percussion decay measure).
func bandDecayMs(x []float64, sr int, lo, hi float64) float64 {
	filt := bandpassNaive(x, sr, lo, hi)
	win := secToSamples(0.006, sr)
	hop := secToSamples(0.002, sr)
	env := kickRMSEnvelope(filt, win, hop)
	if len(env) < 4 {
		return 0
	}
	peakIdx, peak := 0, 0.0
	for i, e := range env {
		if e > peak {
			peak, peakIdx = e, i
		}
	}
	if peak <= 0 {
		return 0
	}
	thr := peak * math.Pow(10, -snareDecayDropDB/20)
	hopSec := float64(hop) / float64(sr)
	for i := peakIdx; i < len(env); i++ {
		if env[i] < thr {
			return float64(i-peakIdx) * hopSec * 1000
		}
	}
	return float64(len(env)-peakIdx) * hopSec * 1000 // never dropped: floor
}

// bandpassNaive is a cheap 2nd-order-ish band limiter (one-pole HP then LP) for
// envelope extraction only (not for audio).
func bandpassNaive(x []float64, sr int, lo, hi float64) []float64 {
	dt := 1.0 / float64(sr)
	rcHP := 1.0 / (2 * math.Pi * lo)
	aHP := rcHP / (rcHP + dt)
	rcLP := 1.0 / (2 * math.Pi * hi)
	aLP := dt / (rcLP + dt)
	out := make([]float64, len(x))
	var hpPrevIn, hpPrevOut, lp float64
	for i, v := range x {
		hp := aHP * (hpPrevOut + v - hpPrevIn)
		hpPrevIn, hpPrevOut = v, hp
		lp += aLP * (hp - lp)
		out[i] = lp
	}
	return out
}

// snareAttackMs is the time from onset (2% of peak) to the envelope peak.
func snareAttackMs(x []float64, sr int) float64 {
	win := secToSamples(0.003, sr)
	hop := secToSamples(0.001, sr)
	env := kickRMSEnvelope(x, win, hop)
	if len(env) < 3 {
		return 0
	}
	peakIdx, peak := 0, 0.0
	for i, e := range env {
		if e > peak {
			peak, peakIdx = e, i
		}
	}
	start := 0
	for i := 0; i <= peakIdx; i++ {
		if env[i] >= 0.02*peak {
			start = i
			break
		}
	}
	return float64(peakIdx-start) * float64(hop) / float64(sr) * 1000
}
