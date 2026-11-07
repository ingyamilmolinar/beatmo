package fingerprint

import (
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Median-of-short-windows YIN (~93 ms frames), so vibrato's period variation cannot smear the CMNDF dip and let a
// subharmonic win; then the spectral octave guard and a snap to the nearest strong peak. 0 if none.
func DetectF0(w wave.Wave) float64 { return DetectF0Detailed(w).Hz }

// The per-frame YIN gate already judges whether a period is trustworthy, which is what separates a pitched instrument
// from a hi-hat that still yields some number; descriptors that need a harmonic series need that judgement.
func DetectF0Detailed(w wave.Wave) F0Estimate {
	// Use the shared sustain window (sustainWindow handles the <4-sample fallback).
	seg := sustainWindow(w)
	if len(seg.Samples) < 4 {
		return F0Estimate{}
	}

	sr := seg.SampleRate
	if sr == 0 {
		return F0Estimate{}
	}
	buf := seg.Samples

	// Check for silence.
	peakAmp := 0.0
	for _, s := range buf {
		if a := math.Abs(s); a > peakAmp {
			peakAmp = a
		}
	}
	if peakAmp < 1e-9 {
		return F0Estimate{}
	}

	// Short enough that a 5 Hz vibrato moves less than half a cycle per frame.
	const frameLen = 4096
	const hop = 2048

	var perFrameF0s []float64
	// A plucked note spends its tail near the floor where YIN rightly gives up; counting those frames equally would score
	// a koto as unpitched.
	totalFrames := 0
	var confidentEnergy, totalEnergy float64
	for start := 0; start+frameLen <= len(buf); start += hop {
		totalFrames++
		frame := buf[start : start+frameLen]
		var frameEnergy float64
		for _, v := range frame {
			frameEnergy += v * v
		}
		totalEnergy += frameEnergy
		f0, dp := yinF0ForFrame(frame, sr)
		if f0 <= 0 {
			continue
		}
		// Transient or noisy frames are dropped.
		if dp < frameConfidenceThreshold {
			perFrameF0s = append(perFrameF0s, f0)
			confidentEnergy += frameEnergy
		}
	}

	// Shorter than one frame: a single run over the whole buffer.
	var yinF0 float64
	// A per-frame median skips the fast-path snap, which can land on a vibrato sideband, and always runs the octave
	// guard.
	haveFrameMedian := false
	var bestDp float64
	if len(perFrameF0s) == 0 {
		// Try the full buffer as a single frame.
		f0, dp := yinF0ForFrame(buf, sr)
		if f0 <= 0 {
			return F0Estimate{}
		}
		yinF0 = f0
		bestDp = dp
	} else {
		// Take the median of per-frame estimates.
		sort.Float64s(perFrameF0s)
		n := len(perFrameF0s)
		if n%2 == 0 {
			yinF0 = (perFrameF0s[n/2-1] + perFrameF0s[n/2]) / 2
		} else {
			yinF0 = perFrameF0s[n/2]
		}
		haveFrameMedian = true
		// Only the non-median path reads bestDp, so the median path skips a redundant full-buffer run.
	}

	// Both the octave guard and the snap need it.
	mag, binHz := wave.MagnitudeSpectrum(seg, 16384, wave.WindowHann)

	// A very confident single-window YIN skips the octave guard, which reintroduces octave errors on dominant-2nd-harmonic
	// instruments whose fundamental sits under the noise floor. Never with a per-frame median.
	const yinConfidenceThreshold = 0.10
	if !haveFrameMedian && bestDp < yinConfidenceThreshold {
		return f0Estimate(snapToSpectralPeak(mag, binHz, yinF0, 0.03), perFrameF0s, totalFrames, confidentEnergy, totalEnergy, bestDp)
	}

	// The candidate among {f/3, f/2, f, 2f, 3f} with the most covered harmonics (f/3 and 3f catch a lock onto the third
	// sub-harmonic). Coverage counts so a sub-octave cannot borrow its parent's energy; ties go to the k=1 energy, then to
	// the candidate nearest YIN.
	candidates := []float64{yinF0 / 3, yinF0 / 2, yinF0, yinF0 * 2, yinF0 * 3}

	// Compute the global spectral noise floor as mean+σ for significance test.
	var magSum, magSumSq float64
	for _, m := range mag {
		magSum += m
		magSumSq += m * m
	}
	magMean := magSum / float64(len(mag))
	magVar := magSumSq/float64(len(mag)) - magMean*magMean
	if magVar < 0 {
		magVar = 0
	}
	noiseFloor := magMean + math.Sqrt(magVar)

	bestCandidate := yinF0 // default: keep YIN estimate
	bestCount := -1
	bestK1Mag := -1.0 // magnitude at k=1 (candidate's own fundamental bin)
	bestTotalEnergy := 0.0

	for _, cand := range candidates {
		// Below 80 Hz leakage from short windows inflates sub-candidates' counts; 3000 Hz admits 3f up to a 1 kHz
		// fundamental.
		if cand < 80 || cand > 3000 {
			continue
		}
		count, energy := harmonicCoverage(mag, binHz, cand, 6, noiseFloor)

		// A true fundamental has energy at its own k=1; a sub-harmonic whose teeth coincide with the real harmonics has little.
		k1Mag := k1Magnitude(mag, binHz, cand)

		// Count, then k=1 energy, then total comb energy, then nearest yinF0.
		better := count > bestCount
		if !better && count == bestCount {
			if k1Mag > bestK1Mag+1e-9 {
				better = true
			} else if math.Abs(k1Mag-bestK1Mag) <= 1e-9 {
				if energy > bestTotalEnergy {
					better = true
				} else if energy == bestTotalEnergy {
					distCurr := math.Abs(bestCandidate - yinF0)
					distNew := math.Abs(cand - yinF0)
					better = distNew < distCurr
				}
			}
		}
		if better {
			bestCount = count
			bestK1Mag = k1Mag
			bestTotalEnergy = energy
			bestCandidate = cand
		}
	}

	// A reed with a weak fundamental (H2 above H1) ties f0 and 2f0 and the k=1 tiebreak picks the octave up; the half
	// frequency's ODD multiples cannot come from the doubled candidate, so with the half frequency present the lower
	// octave wins.
	bestCandidate = halveOnOddHarmonicEvidence(mag, binHz, bestCandidate, noiseFloor)

	// Not after a per-frame median: a vibrato centre sits between sidebands and a ±3 % snap can land on one.
	if haveFrameMedian {
		return f0Estimate(bestCandidate, perFrameF0s, totalFrames, confidentEnergy, totalEnergy, bestDp)
	}
	return f0Estimate(snapToSpectralPeak(mag, binHz, bestCandidate, 0.03), perFrameF0s, totalFrames, confidentEnergy, totalEnergy, bestDp)
}

// Only when the sub-frequency itself is present (a genuinely missing fundamental keeps f0) AND the teeth f0's comb
// cannot explain carry over half the energy of those it can: a weak-fundamental reed measures ~0.75, a cello's
// sub-octave layer ~0.38 and stays put. The f/3 case covers equal-H2/H3 reeds.
func halveOnOddHarmonicEvidence(mag []float64, binHz, f0, noiseFloor float64) float64 {
	for _, div := range []int{3, 2} {
		sub := f0 / float64(div)
		if sub < 50 {
			continue
		}
		if k1Magnitude(mag, binHz, sub) <= noiseFloor {
			continue
		}
		// Only the first 6 sub-grid teeth: they carry the fundamental decision, while a waveguide's period-doubling at high k
		// would erase the separation.
		var newE, oldE float64
		nyq := binHz * float64(len(mag))
		for k := 1; k <= 6 && sub*float64(k) < nyq; k++ {
			m := k1Magnitude(mag, binHz, sub*float64(k))
			if k%div == 0 {
				oldE += m * m
			} else {
				newE += m * m
			}
		}
		if oldE > 0 && newE/oldE > 0.5 {
			return sub
		}
	}
	return f0
}

// (0, 1) when no pitch is found; the frame must be at least 4 samples.
func yinF0ForFrame(buf []float64, sr int) (f0 float64, dp float64) {
	if len(buf) < 4 || sr == 0 {
		return 0, 1
	}

	// Lag range for F0SearchMinHz–F0SearchMaxHz.
	tauMin := sr / F0SearchMaxHz
	tauMax := sr / F0SearchMinHz
	if tauMin < 1 {
		tauMin = 1
	}
	if tauMax > len(buf)/2 {
		tauMax = len(buf) / 2
	}
	if tauMin >= tauMax {
		return 0, 1
	}

	// YIN difference function: d(τ) = Σ (x[t] - x[t+τ])^2
	n := len(buf)
	d := make([]float64, tauMax+1)
	for tau := 1; tau <= tauMax; tau++ {
		sum := 0.0
		for t := 0; t < n-tau; t++ {
			diff := buf[t] - buf[t+tau]
			sum += diff * diff
		}
		d[tau] = sum
	}

	// d'(0) = 1, d'(τ) = d(τ)·τ / Σ_{j=1}^{τ} d(j).
	dpArr := make([]float64, tauMax+1)
	dpArr[0] = 1.0
	runningSum := 0.0
	for tau := 1; tau <= tauMax; tau++ {
		runningSum += d[tau]
		if runningSum > 0 {
			dpArr[tau] = d[tau] * float64(tau) / runningSum
		} else {
			dpArr[tau] = 1.0
		}
	}

	// The first local minimum below the absolute threshold, else the global minimum.
	const threshold = 0.15
	bestTau := -1
	bestDpVal := math.MaxFloat64

	for tau := tauMin; tau <= tauMax-1; tau++ {
		if dpArr[tau] < threshold {
			// Accept if it is a local minimum.
			if dpArr[tau] <= dpArr[tau-1] && dpArr[tau] <= dpArr[tau+1] {
				bestTau = tau
				bestDpVal = dpArr[tau]
				break
			}
		}
	}

	// Fall back to global minimum in range.
	if bestTau < 0 {
		for tau := tauMin; tau <= tauMax; tau++ {
			if dpArr[tau] < bestDpVal {
				bestDpVal = dpArr[tau]
				bestTau = tau
			}
		}
	}

	if bestTau < 0 {
		return 0, 1
	}

	// Parabolic interpolation for sub-sample accuracy.
	tauF := float64(bestTau)
	if bestTau > tauMin && bestTau < tauMax {
		alpha := dpArr[bestTau-1]
		beta := dpArr[bestTau]
		gamma := dpArr[bestTau+1]
		denom := 2 * (alpha - 2*beta + gamma)
		if math.Abs(denom) > 1e-12 {
			delta := (alpha - gamma) / denom
			tauF = float64(bestTau) + delta
			if tauF < float64(tauMin) {
				tauF = float64(tauMin)
			}
			if tauF > float64(tauMax) {
				tauF = float64(tauMax)
			}
		}
	}

	return float64(sr) / tauF, bestDpVal
}

// ±2 bins, so slightly inharmonic or off-bin partials (piano stretch, bells) still count.
func harmonicCoverage(mag []float64, binHz float64, f0 float64, nHarmonics int, noiseFloor float64) (int, float64) {
	const binWindow = 2
	count := 0
	total := 0.0
	for k := 1; k <= nHarmonics; k++ {
		hz := f0 * float64(k)
		centerBin := int(hz/binHz + 0.5)
		lo := centerBin - binWindow
		hi := centerBin + binWindow
		if lo < 0 {
			lo = 0
		}
		if hi >= len(mag) {
			hi = len(mag) - 1
		}
		if lo > hi {
			continue
		}
		// Take the maximum magnitude in the window.
		m := 0.0
		for b := lo; b <= hi; b++ {
			if mag[b] > m {
				m = mag[b]
			}
		}
		total += m
		if m > noiseFloor {
			count++
		}
	}
	return count, total
}

// ±2 bins around the candidate's own fundamental.
func k1Magnitude(mag []float64, binHz float64, f0 float64) float64 {
	const binWindow = 2
	centerBin := int(f0/binHz + 0.5)
	lo := centerBin - binWindow
	hi := centerBin + binWindow
	if lo < 0 {
		lo = 0
	}
	if hi >= len(mag) {
		hi = len(mag) - 1
	}
	if lo > hi {
		return 0
	}
	m := 0.0
	for b := lo; b <= hi; b++ {
		if mag[b] > m {
			m = mag[b]
		}
	}
	return m
}

// f0 unchanged when none is found.
func snapToSpectralPeak(mag []float64, binHz float64, f0 float64, tolFrac float64) float64 {
	loHz := f0 * (1 - tolFrac)
	hiHz := f0 * (1 + tolFrac)
	loBin := int(loHz / binHz)
	hiBin := int(hiHz/binHz) + 1

	if loBin < 0 {
		loBin = 0
	}
	if hiBin >= len(mag) {
		hiBin = len(mag) - 1
	}

	bestBin := -1
	bestMagVal := 0.0
	for i := loBin; i <= hiBin; i++ {
		if mag[i] > bestMagVal {
			bestMagVal = mag[i]
			bestBin = i
		}
	}

	if bestBin < 0 || bestMagVal < 1e-12 {
		return f0
	}
	return float64(bestBin) * binHz
}
