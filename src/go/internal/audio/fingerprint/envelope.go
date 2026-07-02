package fingerprint

import (
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

const envHop = 256

// computeEnvelope fills fp's envelope and timbre-noise fields from w.
//
// The wave w is assumed to already be peak-normalized (as done in FromWave).
// Envelope is computed at hop=256 samples (nPoints=0 → full resolution).
//
// AttackTimeSec: time (seconds) from first env >= 10% of peak to first env
// >= 90% of peak before the peak index. Measured in envelope frames;
// one frame = hop/sampleRate seconds.
//
// LogAttackTime: log10(max(AttackTimeSec, 1e-4)). MPEG-7 LAT.
//
// DecaySlope: least-squares slope (dB/sec) of 20*log10(env) vs time over the
// post-peak region. Negative means decaying. Returns 0 when fewer than 2
// post-peak frames exist.
//
// SustainLevel: median of the middle third of the envelope divided by peak.
// Returns 0 when peak == 0.
//
// ReleaseTimeSec: scanning from the peak to end, find the last frame where
// env >= 90% of the tail peak (the peak in the post-peak region) and the
// last frame where env >= 10% of the tail peak. Release = time between those
// two frames. This is a simple, fully deterministic rule.
//
// HNR (Harmonic-to-Noise Ratio) via real cepstrum on the sustain window:
//   1. Compute magnitude spectrum M[] of the sustain window.
//   2. Log-magnitude: LM[k] = log(M[k] + eps).
//   3. Inverse-DFT (IDFT) of LM[] → real cepstrum C[].
//      We use the IDFT of the symmetric magnitude spectrum (symmetric extension),
//      computed here as a cosine transform of the one-sided log-magnitude:
//      C[n] = (1/M) * Σ_{k=0}^{M-1} lm[k] * cos(π·k·n/(M-1)), M = N/2+1.
//   4. Find the peak quefrency q* in the pitch-relevant band [1/2000 .. 1/50 sec].
//   5. HNR = 10*log10(C[q*]² / residualEnergy) where residualEnergy is the mean
//      squared cepstrum excluding the pitch-quefrency band.
//      Guarded: if residual == 0 or peak energy <= residual, returns 0.
func computeEnvelope(w wave.Wave, fp *Fingerprint) {
	sr := w.SampleRate
	if sr <= 0 {
		sr = 44100
	}
	secPerFrame := float64(envHop) / float64(sr)

	env := RMSEnvelope(w, envHop, 0)
	nFrames := len(env)
	if nFrames == 0 {
		return
	}

	// Find peak frame and value.
	peakIdx := 0
	peakVal := 0.0
	for i, v := range env {
		if v > peakVal {
			peakVal = v
			peakIdx = i
		}
	}

	// ---- AttackTimeSec + LogAttackTime ---------------------------------------
	if peakVal > 0 {
		thresh10 := 0.10 * peakVal
		thresh90 := 0.90 * peakVal

		// Scan pre-peak for first >= 10%.
		first10 := -1
		first90 := -1
		for i := 0; i <= peakIdx; i++ {
			if first10 < 0 && env[i] >= thresh10 {
				first10 = i
			}
			if first10 >= 0 && first90 < 0 && env[i] >= thresh90 {
				first90 = i
			}
		}
		if first10 >= 0 && first90 >= 0 && first90 >= first10 {
			fp.AttackTimeSec = float64(first90-first10) * secPerFrame
		} else if first10 >= 0 {
			// Signal never hit 90% before peak; use distance to peak.
			fp.AttackTimeSec = float64(peakIdx-first10) * secPerFrame
		}
		fp.LogAttackTime = math.Log10(math.Max(fp.AttackTimeSec, 1e-4))
	}

	// ---- DecaySlope (post-peak least-squares dB/sec) -------------------------
	postN := nFrames - peakIdx
	if postN >= 2 {
		// Collect (t, dB) pairs for post-peak frames with positive env.
		decayXs := make([]float64, 0, postN)
		decayYs := make([]float64, 0, postN)
		for i := peakIdx; i < nFrames; i++ {
			v := env[i]
			if v > 1e-12 {
				decayXs = append(decayXs, float64(i)*secPerFrame)
				decayYs = append(decayYs, 20.0*math.Log10(v))
			}
		}
		if len(decayXs) >= 2 {
			fp.DecaySlope = olsSlope(decayXs, decayYs)
		}
	}

	// ---- SustainLevel (median of middle third / peak) ------------------------
	if peakVal > 0 && nFrames >= 3 {
		lo := nFrames / 3
		hi := 2 * nFrames / 3
		if hi <= lo {
			hi = lo + 1
		}
		if hi > nFrames {
			hi = nFrames
		}
		mid := make([]float64, hi-lo)
		copy(mid, env[lo:hi])
		sort.Float64s(mid)
		m := len(mid)
		var med float64
		if m%2 == 0 {
			med = (mid[m/2-1] + mid[m/2]) / 2.0
		} else {
			med = mid[m/2]
		}
		fp.SustainLevel = med / peakVal
	}

	// ---- ReleaseTimeSec ------------------------------------------------------
	// Rule: scan from peak to end to find the "tail peak" (max of post-peak env).
	// Then find the last frame >= 90% of tail peak and the last frame >= 10% of
	// tail peak. Release = (last10 - last90) * secPerFrame.
	// If last90 > last10 (shouldn't happen in normal signals), returns 0.
	if peakIdx < nFrames {
		tailPeak := 0.0
		for i := peakIdx; i < nFrames; i++ {
			if env[i] > tailPeak {
				tailPeak = env[i]
			}
		}
		if tailPeak > 1e-12 {
			thresh90 := 0.90 * tailPeak
			thresh10 := 0.10 * tailPeak
			last90 := -1
			last10 := -1
			for i := peakIdx; i < nFrames; i++ {
				if env[i] >= thresh90 {
					last90 = i
				}
				if env[i] >= thresh10 {
					last10 = i
				}
			}
			if last10 >= 0 && last90 >= 0 && last10 > last90 {
				fp.ReleaseTimeSec = float64(last10-last90) * secPerFrame
			}
		}
	}

	// ---- HNR via real cepstrum -----------------------------------------------
	fp.HNR = computeHNR(w, sr)
}

// computeHNR computes the Harmonic-to-Noise Ratio via the real cepstrum of the
// sustain window's magnitude spectrum.
//
// Formula:
//   1. Extract sustain window (skip SustainSkipSec, take up to SustainLenSec).
//   2. mag[] = MagnitudeSpectrum(seg, 4096, WindowHann) — N/2+1 bins.
//   3. lm[k] = log(mag[k] + eps) for k = 0..N/2.
//   4. Real cepstrum C[n] = (1/M) * Σ_{k=0}^{M-1} lm[k] * cos(π·k·n/(M-1))
//      where M = N/2+1 (the one-sided length); this cosine transform of the
//      one-sided log-magnitude is the real cepstrum used here
//      taken as real part. We compute it via a direct DCT-like sum for correctness.
//   5. Pitch quefrency band: q in [sr/2000 .. sr/50] samples (period band).
//      qLo = ceil(sr/2000 / secPerSample) = ceil(sr/(2000)) quefrency bins.
//      But env is at full resolution: quefrency bin n corresponds to period n/sr.
//      So: qLo = floor(sr/2000), qHi = floor(sr/50).
//   6. peakQ = argmax C[n]² for n in [qLo, qHi].
//   7. residualEnergy = mean(C[n]² for n NOT in [qLo-margin, qHi+margin]).
//   8. HNR = 10*log10(C[peakQ]² / residualEnergy).  Guarded against zeros.
func computeHNR(w wave.Wave, sr int) float64 {
	seg := sustainWindow(w)
	if len(seg.Samples) < 4 {
		return 0
	}

	const fftSize = 4096
	const eps = 1e-12

	mag, _ := wave.MagnitudeSpectrum(seg, fftSize, wave.WindowHann)
	nBins := len(mag) // N/2+1

	// Log-magnitude.
	lm := make([]float64, nBins)
	for i, m := range mag {
		lm[i] = math.Log(m + eps)
	}

	// Real cepstrum via DCT-II style: C[n] = (1/nBins)*Σ_{k=0}^{nBins-1} lm[k]*cos(π*k*n/(nBins-1))
	// This is the real cepstrum of the one-sided log-magnitude spectrum.
	// We need quefrency indices n = 0..nBins-1.
	// Quefrency n corresponds to period n/(sr) seconds (at sample resolution sr).
	// sr/2000 = qLo, sr/50 = qHi.
	qLo := sr / 2000 // e.g. 44100/2000 = 22
	qHi := sr / 50   // e.g. 44100/50  = 882

	// Clamp to available range.
	if qHi >= nBins {
		qHi = nBins - 1
	}
	if qLo < 1 {
		qLo = 1
	}
	if qLo > qHi {
		return 0
	}

	// Compute only the cepstrum quefrency values we need (pitch band + a sample
	// of the residual band).  For large nBins a full O(N²) sum is expensive, so
	// we compute the cepstrum for [1..qHi+1] and a sample beyond.
	//
	// However for correctness with fftSize=4096 → nBins=2049 and qHi≈882 this
	// is at most O(882*2049) ≈ 1.8M ops — acceptable for a one-shot analysis.
	// We compute C[n] for n = 0 .. min(nBins-1, qHi+qLo).
	maxQ := qHi + qLo + 1
	if maxQ >= nBins {
		maxQ = nBins - 1
	}

	cep := make([]float64, maxQ+1)
	scale := 1.0 / float64(nBins)
	nm1 := float64(nBins - 1)
	for n := 0; n <= maxQ; n++ {
		sum := 0.0
		fn := float64(n)
		for k := 0; k < nBins; k++ {
			sum += lm[k] * math.Cos(math.Pi*float64(k)*fn/nm1)
		}
		cep[n] = sum * scale
	}

	// Find peak in pitch band.
	peakEnergy := 0.0
	for n := qLo; n <= qHi && n <= maxQ; n++ {
		e := cep[n] * cep[n]
		if e > peakEnergy {
			peakEnergy = e
		}
	}

	// Residual: mean squared cepstrum outside the pitch band (use n=1..qLo-1).
	residSum := 0.0
	residN := 0
	for n := 1; n < qLo && n <= maxQ; n++ {
		residSum += cep[n] * cep[n]
		residN++
	}
	if residN == 0 || residSum <= 0 {
		return 0
	}
	residMean := residSum / float64(residN)
	if peakEnergy <= residMean {
		return 0
	}
	return 10.0 * math.Log10(peakEnergy/residMean)
}

// LogSpectralDistance returns the mean |20*log10(a[i]/b[i])| over all bins.
// Both a and b must be linear magnitude spectra of the same length.
// Each bin is floored at eps before taking the ratio to avoid log(0).
// Used by Distance (Task 9).
func LogSpectralDistance(a, b []float64) float64 {
	n := len(a)
	if n == 0 || len(b) != n {
		return 0
	}
	const eps = 1e-12
	sum := 0.0
	for i := 0; i < n; i++ {
		ai := a[i]
		if ai < eps {
			ai = eps
		}
		bi := b[i]
		if bi < eps {
			bi = eps
		}
		sum += math.Abs(20.0 * math.Log10(ai/bi))
	}
	return sum / float64(n)
}

// SpectralConvergence returns ||a-b||_2 / ||a||_2 (linear magnitude spectra,
// same length). Returns 0 when a == b exactly (or both are zero).
// Used by Distance (Task 9).
func SpectralConvergence(a, b []float64) float64 {
	n := len(a)
	if n == 0 || len(b) != n {
		return 0
	}
	numSq := 0.0
	denomSq := 0.0
	for i := 0; i < n; i++ {
		d := a[i] - b[i]
		numSq += d * d
		denomSq += a[i] * a[i]
	}
	if denomSq < 1e-24 {
		return 0
	}
	return math.Sqrt(numSq / denomSq)
}
