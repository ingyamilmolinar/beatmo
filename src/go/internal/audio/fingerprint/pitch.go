package fingerprint

import (
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// DetectF0 estimates the fundamental frequency (Hz) of w. It uses a
// median-of-short-windows YIN approach for vibrato robustness: the sustain
// window is sliced into overlapping ~93 ms frames (4096 samples at 44.1 kHz),
// each frame produces a YIN CMNDF period estimate, and the median of all
// per-frame estimates is taken as the robust f0. This prevents the full-window
// YIN from being fooled by vibrato-induced period variation, which smears the
// CMNDF dip and lets a subharmonic lag win.
//
// The median f0 is then refined via the existing spectral octave guard (among
// {f/2, f, 2f} pick the one with the most harmonic coverage in the full-window
// spectrum) and snapped to the nearest strong spectral peak for sub-bin
// accuracy. Returns 0 if no pitch is found.
func DetectF0(w wave.Wave) float64 {
	// Use the shared sustain window (sustainWindow handles the <4-sample fallback).
	seg := sustainWindow(w)
	if len(seg.Samples) < 4 {
		return 0
	}

	sr := seg.SampleRate
	if sr == 0 {
		return 0
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
		return 0
	}

	// --- Median-of-short-windows YIN ---
	// Frame size ≈ 93 ms at 44.1 kHz — short enough that a 5 Hz vibrato
	// (period ~200 ms) moves the pitch by less than half a cycle per frame,
	// so within-frame period variation is negligible. Hop = frameLen/2.
	const frameLen = 4096
	const hop = 2048

	var perFrameF0s []float64
	for start := 0; start+frameLen <= len(buf); start += hop {
		frame := buf[start : start+frameLen]
		f0, dp := yinF0ForFrame(frame, sr)
		if f0 <= 0 {
			continue
		}
		// Reject frames where YIN is not confident — these are likely
		// transient or noisy segments. Keep confident estimates only.
		const frameConfidenceThreshold = 0.25
		if dp < frameConfidenceThreshold {
			perFrameF0s = append(perFrameF0s, f0)
		}
	}

	// If the buffer is shorter than one frame, fall back to a single-frame run
	// on the full buffer (handles short test signals and edge cases).
	var yinF0 float64
	// haveFrameMedian indicates that yinF0 comes from a robust per-frame median
	// rather than a single full-window run. When true, we skip the fast-path
	// spectral snap (which can snap to vibrato sidebands instead of the center
	// frequency) and always run the octave guard for a more reliable result.
	haveFrameMedian := false
	var bestDp float64
	if len(perFrameF0s) == 0 {
		// Try the full buffer as a single frame.
		f0, dp := yinF0ForFrame(buf, sr)
		if f0 <= 0 {
			return 0
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
		// bestDp stays zero here: it is only consulted on the non-median path
		// (guarded by !haveFrameMedian below), so the per-frame median path does
		// not pay for a redundant full-buffer YIN run.
	}

	// Compute magnitude spectrum — needed for both the optional octave guard
	// and the final snap-to-peak refinement regardless of which path is taken.
	mag, binHz := wave.MagnitudeSpectrum(seg, 16384, wave.WindowHann)

	// YIN-confidence gate: when d'(τ) is very low AND we did not use per-frame
	// median (which is always more reliable for vibrato signals), the period
	// estimate is highly reliable. Skip the spectral octave guard in that case —
	// overriding a confident single-window YIN with spectral heuristics
	// reintroduces octave errors on dominant-2nd-harmonic instruments (e.g.
	// piano, guitar) where the fundamental bin falls below the spectral noise
	// floor.
	//
	// We do NOT apply this shortcut when haveFrameMedian is true: the
	// per-frame median is a better estimate of the center frequency for vibrato
	// signals, and the spectral snap in the fast path can accidentally snap to
	// a vibrato sideband (e.g. f0-vibHz) instead of the true center.
	const yinConfidenceThreshold = 0.10
	if !haveFrameMedian && bestDp < yinConfidenceThreshold {
		return snapToSpectralPeak(mag, binHz, yinF0, 0.03)
	}

	// Spectral octave guard: among {f/3, f/2, f, 2f, 3f}, pick the candidate
	// with the highest harmonic coverage count. Extending to f/3 and 3f catches
	// the case where YIN locks onto a 1/3 sub-harmonic: when yinF0 ≈ f0/3, the
	// old {f/2, f, 2f} candidate list never tested 3×yinF0 = f0.
	//
	// Coverage count avoids the problem of a sub-octave candidate "borrowing"
	// its parent's energy at k=2: a pure 220 Hz tone gives both 110 Hz and
	// 220 Hz a single strong harmonic, so they tie — and we keep YIN's answer.
	// For a missing-fundamental signal (440+660+880), 220 Hz has 3 harmonics
	// covered while 110 Hz has only 2, so 220 Hz wins.
	//
	// On count ties the secondary tiebreak is the k=1 magnitude (energy at the
	// candidate's own fundamental bin). When two candidates cover the same number
	// of harmonics, the one whose comb starts with real energy at k=1 is more
	// likely to be the true fundamental (vs. a sub-harmonic whose comb teeth
	// happen to land on the true fundamental's harmonics). Tertiary: prefer the
	// candidate closest to yinF0, preserving the YIN estimate when spectral
	// evidence is ambiguous.
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
		// Reject candidates outside a sane frequency range for pitched instruments.
		// Lower bound of 80 Hz avoids f/3 sub-candidates that fall into the
		// sub-bass range where spectral leakage from short windows can spuriously
		// inflate harmonic counts. Upper bound of 3000 Hz allows the 3f candidate
		// for fundamentals up to 1000 Hz (covering the full melodic range).
		if cand < 80 || cand > 3000 {
			continue
		}
		count, energy := harmonicCoverage(mag, binHz, cand, 6, noiseFloor)

		// Compute k=1 magnitude: the magnitude at the candidate's own fundamental
		// bin. A true fundamental always has energy at k=1; a sub-harmonic whose
		// comb teeth coincide with the true fundamental's harmonics typically has
		// little or no energy at its own k=1 position.
		k1Mag := k1Magnitude(mag, binHz, cand)

		// Primary: harmonic count. Secondary (count tie): k=1 magnitude (more
		// energy at the candidate's own fundamental = more likely a true pitch).
		// Tertiary: total comb energy. Quaternary: prefer candidate nearest yinF0.
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

	// Odd-harmonic subharmonic evidence: a rich reed/brass spectrum with a
	// weak fundamental (e.g. baritone sax — H2 ~7 dB ABOVE H1, full 12+
	// harmonic series) makes the 6-harmonic coverage count TIE between f0 and
	// 2f0, and the k1Mag tiebreak then picks the louder 2nd harmonic — an
	// octave-up error. The half-frequency candidate's ODD multiples (1, 3,
	// 5 × f0/2) cannot be explained by the doubled candidate; when the half
	// frequency itself is present AND another odd multiple is above the noise
	// floor, the lower octave is the true fundamental.
	bestCandidate = halveOnOddHarmonicEvidence(mag, binHz, bestCandidate, noiseFloor)

	// Snap to nearest strong spectral peak within ±3% for refinement.
	//
	// When the estimate comes from a per-frame median (haveFrameMedian), the
	// center frequency of a vibrato signal sits between sidebands (e.g. f0±vibHz).
	// A simple ±3% snap can land on a sideband rather than the true center,
	// degrading accuracy. Skip the snap in that case — the per-frame median plus
	// the octave guard already produced a reliable center-frequency estimate.
	if haveFrameMedian {
		return bestCandidate
	}
	return snapToSpectralPeak(mag, binHz, bestCandidate, 0.03)
}

// halveOnOddHarmonicEvidence returns f0/3 or f0/2 when the spectrum contains
// real energy at that sub-frequency AND the sub-grid's "new" harmonics (the
// teeth the detected f0's comb cannot account for) carry energy COMPARABLE to
// the accounted-for teeth. Requirements:
//
//   - The sub-frequency itself (the true weak fundamental) must be present, so
//     genuinely missing-fundamental signals keep the detected f0.
//   - newE/oldE > 0.5: the unexplained teeth must carry at least half the
//     energy of the explained ones. A true weak-fundamental reed series (bari
//     sax: H1 ~7 dB below H2, H3 ≈ H2) measures ~0.75 here; a sub-octave
//     LAYER (the cello's bowed waveguide + sub gen slot at D2: half-grid teeth
//     at 36-83% but odd-dominated-DOWN) measures ~0.38 and must NOT drop the
//     octave — the note is still D2.
//
// The f/3 case covers equal-H2/H3 reed spectra where YIN locks onto the H3
// period and the k1Mag tiebreak keeps it.
func halveOnOddHarmonicEvidence(mag []float64, binHz, f0, noiseFloor float64) float64 {
	for _, div := range []int{3, 2} {
		sub := f0 / float64(div)
		if sub < 50 {
			continue
		}
		if k1Magnitude(mag, binHz, sub) <= noiseFloor {
			continue
		}
		// Only the first 6 sub-grid teeth vote: the low harmonics carry the
		// perceptual fundamental decision (bari sax scores ~0.69 there vs the
		// cello sub-layer's ~0.34), while a bowed waveguide's period-doubling
		// components at high k would pollute a wider window (12 teeth read
		// ~0.73 for BOTH cases — no separation).
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

// yinF0ForFrame runs the YIN CMNDF algorithm on a single audio frame and
// returns (f0Hz, bestDp) where bestDp is the normalized difference value at
// the chosen lag. Returns (0, 1) if no pitch is detected. The frame must be
// at least 4 samples long.
func yinF0ForFrame(buf []float64, sr int) (f0 float64, dp float64) {
	if len(buf) < 4 || sr == 0 {
		return 0, 1
	}

	// Lag range for 50–2000 Hz.
	tauMin := sr / 2000
	tauMax := sr / 50
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

	// Cumulative mean normalized difference function d'(τ):
	// d'(0) = 1, d'(τ) = d(τ) * τ / Σ_{j=1}^{τ} d(j)
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

	// Find the first local minimum in [tauMin, tauMax] where dp drops below
	// the absolute threshold. If none found, fall back to the global minimum.
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

// harmonicCoverage returns (count, totalMag) for the first nHarmonics integer
// multiples of f0. count is the number of harmonics whose magnitude exceeds
// noiseFloor. totalMag is the sum of magnitudes at all harmonic bins.
//
// A ±2-bin window is used around each target bin so that slightly inharmonic
// or off-bin partials (e.g. piano stretched tuning, bell inharmonicity) still
// register as covered rather than being silently missed.
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

// k1Magnitude returns the maximum magnitude within a ±2-bin window around the
// k=1 (fundamental) bin of f0. This is the energy at the candidate's own
// pitch frequency. A true fundamental has real energy here; a sub-harmonic
// whose comb teeth coincide with the true fundamental's harmonics typically
// has little or no energy at its own k=1 position.
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

// snapToSpectralPeak finds the strongest spectral peak within ±tolFrac of f0
// and returns that frequency if found, otherwise returns f0.
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
