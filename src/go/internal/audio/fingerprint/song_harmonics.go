package fingerprint

import "math"

// HarmonicPeak is one spectral peak (frequency + magnitude).
type HarmonicPeak struct{ Hz, Mag float64 }

// harmonicMinHz floors peak detection: sub-audio rumble / DC / room noise below
// this is ignored so PrimaryHz is always a plausible musical pitch. 25 Hz sits
// just below the lowest piano note (A0 ≈ 27.5 Hz) so no real fundamental is lost.
const harmonicMinHz = 25.0

// HarmonicProfile returns the n strongest local-maximum peaks, descending by
// magnitude. peaks[0] is the primary; the rest are secondary harmonics/partials.
// Peaks below harmonicMinHz are ignored.
func HarmonicProfile(mag []float64, binHz float64, n int) []HarmonicPeak {
	var peaks []HarmonicPeak
	for i := 1; i < len(mag)-1; i++ {
		if float64(i)*binHz < harmonicMinHz {
			continue
		}
		if mag[i] >= mag[i-1] && mag[i] >= mag[i+1] {
			peaks = append(peaks, HarmonicPeak{Hz: float64(i) * binHz, Mag: mag[i]})
		}
	}
	// Descending sort by magnitude (insertion sort; n is small).
	for i := 1; i < len(peaks); i++ {
		k := peaks[i]
		j := i - 1
		for j >= 0 && peaks[j].Mag < k.Mag {
			peaks[j+1] = peaks[j]
			j--
		}
		peaks[j+1] = k
	}
	if n < len(peaks) {
		peaks = peaks[:n]
	}
	return peaks
}

// IntervalHistogram is the circular autocorrelation of chroma: how much energy
// sits at each pitch-class interval (index 0 = unison, 7 = perfect fifth, …).
func IntervalHistogram(chroma [12]float64) [12]float64 {
	var h [12]float64
	for lag := 0; lag < 12; lag++ {
		s := 0.0
		for i := 0; i < 12; i++ {
			s += chroma[i] * chroma[(i+lag)%12]
		}
		h[lag] = s
	}
	return h
}

// IntonationCents returns the magnitude-weighted mean cents deviation of strong
// peaks from the nearest equal-tempered semitone (A4=440). Positive = sharp.
func IntonationCents(mag []float64, binHz float64) float64 {
	if binHz <= 0 {
		return 0
	}
	// Mean magnitude as a peak-significance floor.
	mean := 0.0
	for _, m := range mag {
		mean += m
	}
	mean /= float64(len(mag))
	var wsum, wcents float64
	for i := 1; i < len(mag)-1; i++ {
		if mag[i] < mean*4 || !(mag[i] >= mag[i-1] && mag[i] >= mag[i+1]) {
			continue
		}
		f := float64(i) * binHz
		if f < 20 || f > 5000 {
			continue
		}
		midi := 69 + 12*math.Log2(f/440.0)
		cents := (midi - math.Round(midi)) * 100
		wsum += mag[i]
		wcents += mag[i] * cents
	}
	if wsum == 0 {
		return 0
	}
	return wcents / wsum
}
