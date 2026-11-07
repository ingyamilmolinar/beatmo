package fingerprint

import (
	"fmt"
	"math"
)

var octaveBandCentersHz = []float64{125, 250, 500, 1000, 2000, 4000, 8000}
var octaveBandLabels = []string{"125", "250", "500", "1k", "2k", "4k", "8k"}

const bandDecayFitFloorDB = 40.0

// A band whose own peak is this far below the loudest band's carries no energy: an OLS slope over near-silence measures
// leakage, not decay.
const bandEnergyGateDB = 30.0

// -1, since a real T60 is always > 0; unlike NaN it keeps every OK measurement finite.
const bandT60Unfit = -1.0

type BandDecay struct {
	T60Sec       []float64
	OK           []bool
	RateDBPerSec []float64
}

// Fits each octave's decay RATE from the visible tail, so a clip ending above -60 dB still yields a number.
func BandT60(st *STFTFrames) BandDecay {
	n := len(octaveBandCentersHz)
	out := BandDecay{T60Sec: make([]float64, n), OK: make([]bool, n), RateDBPerSec: make([]float64, n)}
	if st == nil || len(st.Frames) < 3 {
		return out
	}
	// The energy gate needs every band's peak first, so tracks and peaks come before the fits.
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
