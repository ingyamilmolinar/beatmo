package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

const shapeEps = 1e-12

// computeShape fills fp's spectral-shape fields from w's sustain-window
// magnitude spectrum and fp.Partials (already populated by computeSpectral).
// Sustain window policy: skip SustainSkipSec, analyze SustainLenSec; fall back
// to the whole wave when the window is too short.
func computeShape(w wave.Wave, fp *Fingerprint) {
	// Sustain window — shared policy via sustainWindow.
	seg := sustainWindow(w)
	if len(seg.Samples) < 4 {
		return
	}

	mag, binHz := wave.MagnitudeSpectrum(seg, 16384, wave.WindowHann)

	// Work over bins i>=1 (skip DC).
	// -----------------------------------------------------------------------
	// SpectralFlatness = geomean(mag) / arithmean(mag).
	// geomean via exp(mean(log(mag+eps))).
	// -----------------------------------------------------------------------
	n := len(mag) - 1 // number of non-DC bins
	if n <= 0 {
		return
	}

	logSum := 0.0
	arithSum := 0.0
	maxMag := 0.0
	for i := 1; i < len(mag); i++ {
		m := mag[i]
		logSum += math.Log(m + shapeEps)
		arithSum += m
		if m > maxMag {
			maxMag = m
		}
	}
	arithMean := arithSum / float64(n)

	geomMean := math.Exp(logSum / float64(n))

	if arithMean > shapeEps {
		fp.SpectralFlatness = geomMean / arithMean
	}

	// -----------------------------------------------------------------------
	// SpectralCrest = max(mag) / arithmean(mag).
	// -----------------------------------------------------------------------
	if arithMean > shapeEps {
		fp.SpectralCrest = maxMag / arithMean
	}

	// -----------------------------------------------------------------------
	// SpectralSkewness and SpectralKurtosis: weighted moments over frequency.
	// w_i = mag[i], f_i = i * binHz.
	// μ = Σ w_i f_i / W,  σ² = Σ w_i (f_i−μ)² / W,
	// skew = Σ w_i (f_i−μ)³ / (W σ³),  kurt = Σ w_i (f_i−μ)⁴ / (W σ⁴).
	// -----------------------------------------------------------------------
	W := arithSum // Σ w_i (same as arithSum computed above)
	if W > shapeEps {
		// Weighted mean frequency.
		wMeanHz := 0.0
		for i := 1; i < len(mag); i++ {
			wMeanHz += mag[i] * float64(i) * binHz
		}
		wMeanHz /= W

		// Weighted variance.
		wVar := 0.0
		for i := 1; i < len(mag); i++ {
			d := float64(i)*binHz - wMeanHz
			wVar += mag[i] * d * d
		}
		wVar /= W

		if wVar > shapeEps {
			sigma := math.Sqrt(wVar)
			sigma3 := sigma * sigma * sigma
			sigma4 := sigma3 * sigma

			skewNum := 0.0
			kurtNum := 0.0
			for i := 1; i < len(mag); i++ {
				d := float64(i)*binHz - wMeanHz
				d2 := d * d
				skewNum += mag[i] * d * d2
				kurtNum += mag[i] * d2 * d2
			}
			fp.SpectralSkewness = skewNum / (W * sigma3)
			fp.SpectralKurtosis = kurtNum / (W * sigma4)
		}
	}

	// -----------------------------------------------------------------------
	// Tristimulus from fp.Partials (squared partial amplitudes p_k²).
	// Indices: k=0..15 → partials 1..16.
	// T1 = p1² / Σ
	// T2 = (p2²+p3²+p4²) / Σ
	// T3 = (p5²+…+p16²) / Σ
	// -----------------------------------------------------------------------
	var sumSq float64
	var pSq [16]float64
	for k := 0; k < 16; k++ {
		pSq[k] = fp.Partials[k] * fp.Partials[k]
		sumSq += pSq[k]
	}
	if sumSq > shapeEps {
		fp.Tristimulus[0] = pSq[0] / sumSq
		t2 := pSq[1] + pSq[2] + pSq[3]
		fp.Tristimulus[1] = t2 / sumSq
		t3 := 0.0
		for k := 4; k < 16; k++ {
			t3 += pSq[k]
		}
		fp.Tristimulus[2] = t3 / sumSq
	}
}
