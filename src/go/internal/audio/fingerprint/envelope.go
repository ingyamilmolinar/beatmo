package fingerprint

import (
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

const envHop = 256

// w is already peak-normalised; envelope hop 256. Attack is 10 % → 90 % of the peak (MPEG-7 log attack), release the
// last 90 % → 10 % of the post-peak tail peak, and HNR comes from the real cepstrum of the sustain window, guarded
// against zero residual.
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

	// 0 if last90 > last10, which a normal signal never produces.
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

// The cepstrum's peak in the pitch quefrency band [sr/2000, sr/50] against the mean energy outside it, guarded against
// zeros.
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

	// A cosine transform of the one-sided log magnitude; quefrency n is a period of n/sr seconds.
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

	// Only the quefrencies needed (the pitch band and a residual sample): a full O(N²) sum is expensive.
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

// Same-length linear spectra; each bin is floored at eps.
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

// 0 when equal or both zero.
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
