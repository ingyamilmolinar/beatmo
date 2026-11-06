package fingerprint

import (
	"fmt"
	"math"
)

var octaveBandCentersHz = []float64{125, 250, 500, 1000, 2000, 4000, 8000}
var octaveBandLabels = []string{"125", "250", "500", "1k", "2k", "4k", "8k"}

const bandDecayFitFloorDB = 40.0

// bandEnergyGateDB: a band whose own peak sits more than this far below the
// LOUDEST band's peak, in the SAME signal, carries essentially no energy and
// must never be "fitted" -- an OLS slope over near-silence measures FFT
// leakage/noise floor drift, not a real decay. Without this gate, a harp's
// fundamental-and-above spectrum still reported a "measurable" 125 Hz and
// 500 Hz band T60 purely from spectral leakage of the real (much higher)
// partials, both sitting BELOW the instrument's actual fundamental with
// almost no energy of their own -- see TestBandT60_EnergyGateRejectsQuietBands.
const bandEnergyGateDB = 30.0

// bandT60Unfit is the sentinel Vector component for a band whose decay could
// not be fitted (BandDecay.OK[i]==false). A real T60Sec is always > 0, so -1
// is unambiguous and, unlike math.NaN(), keeps every Extract-ed Measurement
// finite -- required by TestNoDescriptorProducesNonFiniteValues and
// TestFamilyExtractionIsFinite, which forbid NaN/Inf in ANY OK measurement.
const bandT60Unfit = -1.0

type BandDecay struct {
	T60Sec       []float64
	OK           []bool
	RateDBPerSec []float64
}

// BandT60 fits each octave band's decay RATE from the visible tail and
// converts it to a T60, so a clip that ends above -60 dB still yields a
// number. This is the replacement for the clamped env.ring_t60.
func BandT60(st *STFTFrames) BandDecay {
	n := len(octaveBandCentersHz)
	out := BandDecay{T60Sec: make([]float64, n), OK: make([]bool, n), RateDBPerSec: make([]float64, n)}
	if st == nil || len(st.Frames) < 3 {
		return out
	}
	// First pass: build each band's dB-over-time track and find its own
	// peak. The energy gate below needs every band's peak before it can
	// decide which ones are loud enough relative to the LOUDEST band in
	// this same signal, so the fit loop cannot run in the same pass.
	tracks := make([][]float64, n)
	peaks := make([]float64, n)
	peakFrames := make([]int, n)
	loudestPeak := math.Inf(-1)
	for bi, c := range octaveBandCentersHz {
		lo, hi := c/math.Sqrt2, c*math.Sqrt2
		track := make([]float64, len(st.Frames))
		for fi, mag := range st.Frames {
			e := 0.0
			for k := int(lo / st.BinHz); k <= int(hi/st.BinHz) && k < len(mag); k++ {
				if k >= 1 {
					e += mag[k] * mag[k]
				}
			}
			track[fi] = 10 * math.Log10(e+1e-20)
		}
		peak, pi := math.Inf(-1), 0
		for i, v := range track {
			if v > peak {
				peak, pi = v, i
			}
		}
		tracks[bi], peaks[bi], peakFrames[bi] = track, peak, pi
		if peak > loudestPeak {
			loudestPeak = peak
		}
	}

	for bi := range octaveBandCentersHz {
		peak, pi, track := peaks[bi], peakFrames[bi], tracks[bi]
		if peak < -100 || peak < loudestPeak-bandEnergyGateDB {
			continue
		}
		var xs, ys []float64
		for i := pi; i < len(track); i++ {
			if track[i] < peak-bandDecayFitFloorDB {
				break
			}
			xs = append(xs, float64(i-pi)*st.HopSec)
			ys = append(ys, track[i])
		}
		if len(xs) < 3 {
			continue
		}
		rate := olsSlope(xs, ys)
		if rate >= -1e-6 {
			continue
		}
		out.RateDBPerSec[bi] = rate
		out.T60Sec[bi] = 60 / -rate
		out.OK[bi] = true
	}
	return out
}

func PerModeDecay(ms *ModeSet, n int) (rates []float64, labels []string, ok bool) {
	for _, m := range ms.Strong(modeStrongWithin) {
		if !m.DecayOK {
			continue
		}
		rates = append(rates, m.DecayDBPerSec)
		labels = append(labels, fmt.Sprintf("m%d", len(rates)))
		if len(rates) >= n {
			break
		}
	}
	return rates, labels, len(rates) >= 1
}

func DecaySlopeVsFreq(ms *ModeSet) (float64, bool) {
	var xs, ys []float64
	for _, m := range ms.Strong(modeStrongWithin) {
		if m.DecayOK {
			xs = append(xs, m.Hz/1000)
			ys = append(ys, m.DecayDBPerSec)
		}
	}
	if len(xs) < 3 {
		return 0, false
	}
	return olsSlope(xs, ys), true
}
