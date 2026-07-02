package fingerprint

import (
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Formant describes a single resonance peak in the spectral envelope.
type Formant struct {
	Hz          float64
	BandwidthHz float64 // -3 dB width around the peak
	GainDB      float64 // relative to the envelope maximum (≤ 0)
}

// SpectralEnvelope summarises the smoothed spectral envelope and its peaks.
type SpectralEnvelope struct {
	Formants         []Formant // strongest peaks, descending GainDB
	BridgeHillHz     float64   // dominant peak in 1.8–3.3 kHz (canonical violin bridge-hill region); 0 if absent
	BridgeHillGainDB float64
}

const (
	formantLoHz    = 100.0
	formantHiHz    = 8000.0
	bridgeHillLoHz = 1800.0
	bridgeHillHiHz = 3300.0
	// bandwidthWalkMaxHz caps each half of the -3 dB bandwidth walk (~800 Hz
	// per side) to prevent multi-kHz bogus estimates when the envelope never
	// drops 3 dB (e.g. a broad or monotone peak).
	bandwidthWalkMaxHz = 800.0
	// bandwidthMaxHz clamps the final computed BandwidthHz to a sane upper bound.
	bandwidthMaxHz = 2000.0
	// cepstral lifter: keep only quefrencies 0..cepLifterN-1 (smooth envelope).
	cepLifterN = 50
	// minimum log-magnitude power to avoid log(0).
	formantEps = 1e-10
	// formants more than this many dB below the envelope peak are noise floor artifacts.
	formantMinGainDB = -60.0
)

// computeSpectralEnvelope estimates the spectral envelope of w via real-cepstrum
// liftering, then peak-picks formants and identifies the violin "bridge hill".
// Returns a zero SpectralEnvelope when w is silent or too short.
func computeSpectralEnvelope(w wave.Wave, cfg AnalysisConfig) SpectralEnvelope {
	if len(w.Samples) < 4 || w.SampleRate == 0 {
		return SpectralEnvelope{}
	}

	// Silence guard: skip if peak amplitude is negligible.
	peak := 0.0
	for _, s := range w.Samples {
		if v := math.Abs(s); v > peak {
			peak = v
		}
	}
	if peak < formantEps {
		return SpectralEnvelope{}
	}

	fftSize := cfg.FFTSize
	if fftSize <= 0 {
		fftSize = mfccFFTSize
	}

	// 1. Compute linear magnitude spectrum.
	mag, binHz := wave.MagnitudeSpectrum(w, fftSize, wave.WindowHann)
	numBins := len(mag)

	// 2. Log-magnitude spectrum.
	logMag := make([]float64, numBins)
	for i, m := range mag {
		if m < formantEps {
			m = formantEps
		}
		logMag[i] = math.Log(m)
	}

	// 3. Real-cepstrum liftering via DCT-II (forward) + zero high quefrencies + inverse DCT.
	// Forward DCT-II of logMag → cepstrum coefficients.
	cep := make([]float64, numBins)
	dctII(logMag, cep)

	// Zero out high-quefrency coefficients (liftering: keep 0..cepLifterN-1).
	for i := cepLifterN; i < numBins; i++ {
		cep[i] = 0
	}

	// Inverse DCT-II (= DCT-III scaled): reconstruct smooth log-envelope.
	// IDCT-II(x)[n] = (x[0] + 2*sum_{k=1}^{N-1} x[k]*cos(pi/N*(n+0.5)*k)) / N
	envelope := make([]float64, numBins)
	N := float64(numBins)
	piOverN := math.Pi / N
	for n := 0; n < numBins; n++ {
		sum := cep[0]
		for k := 1; k < cepLifterN && k < numBins; k++ {
			sum += 2.0 * cep[k] * math.Cos(piOverN*float64(k)*(float64(n)+0.5))
		}
		envelope[n] = sum / N
	}

	// 4. Find the global maximum of the envelope for relative gain calculation.
	envMax := -math.MaxFloat64
	for _, v := range envelope {
		if v > envMax {
			envMax = v
		}
	}

	// 5. Peak-pick local maxima of the envelope in [formantLoHz, formantHiHz].
	loIdx := int(math.Ceil(formantLoHz / binHz))
	hiIdx := int(math.Floor(formantHiHz / binHz))
	if loIdx < 1 {
		loIdx = 1
	}
	if hiIdx >= numBins-1 {
		hiIdx = numBins - 2
	}

	var formants []Formant
	for i := loIdx; i <= hiIdx; i++ {
		if envelope[i] <= envelope[i-1] || envelope[i] <= envelope[i+1] {
			continue // not a local maximum
		}

		hz := float64(i) * binHz
		gainDB := 20.0 * (envelope[i] - envMax) // relative gain (≤ 0 dB)
		if gainDB < formantMinGainDB {
			continue // noise floor; skip
		}

		// -3 dB bandwidth: walk left and right from the peak until we drop 3 dB
		// in log-magnitude (= log(√2) ≈ 0.3466 in natural log units, since
		// envelope is log(|X|), not log(|X|²)). -3 dB in amplitude → factor √2
		// → Δlog = log(√2).
		// The walk is bounded by three stopping conditions per side:
		//   (a) the envelope crosses the -3 dB threshold,
		//   (b) an adjacent local minimum of the envelope (prevents leaking into
		//       a neighboring peak),
		//   (c) a sane half-window of bandwidthWalkMaxHz.
		thresh := envelope[i] - math.Log(math.Sqrt2)
		maxWalkBins := int(bandwidthWalkMaxHz/binHz + 0.5)
		loHalf := i
		for loHalf > 1 && envelope[loHalf] >= thresh {
			// Stop at a true local minimum: the value at loHalf is lower than
			// both its neighbours (a valley between two peaks), not merely the
			// downslope from the current peak.
			if loHalf < i-1 && envelope[loHalf] <= envelope[loHalf-1] && envelope[loHalf] <= envelope[loHalf+1] {
				break
			}
			if i-loHalf >= maxWalkBins {
				break
			}
			loHalf--
		}
		hiHalf := i
		for hiHalf < numBins-2 && envelope[hiHalf] >= thresh {
			// Stop at a true local minimum: the value at hiHalf is lower than
			// both its neighbours.
			if hiHalf > i+1 && envelope[hiHalf] <= envelope[hiHalf-1] && envelope[hiHalf] <= envelope[hiHalf+1] {
				break
			}
			if hiHalf-i >= maxWalkBins {
				break
			}
			hiHalf++
		}
		bw := float64(hiHalf-loHalf) * binHz
		if bw > bandwidthMaxHz {
			bw = bandwidthMaxHz
		}

		formants = append(formants, Formant{
			Hz:          hz,
			BandwidthHz: bw,
			GainDB:      gainDB,
		})
	}

	// 6. Sort formants by GainDB descending (strongest first).
	sort.Slice(formants, func(i, j int) bool {
		return formants[i].GainDB > formants[j].GainDB
	})

	// 7. Bridge hill: strongest formant whose Hz is in [bridgeHillLoHz, bridgeHillHiHz].
	var bridgeHz, bridgeGainDB float64
	for _, f := range formants {
		if f.Hz >= bridgeHillLoHz && f.Hz <= bridgeHillHiHz {
			bridgeHz = f.Hz
			bridgeGainDB = f.GainDB
			break
		}
	}

	return SpectralEnvelope{
		Formants:         formants,
		BridgeHillHz:     bridgeHz,
		BridgeHillGainDB: bridgeGainDB,
	}
}
