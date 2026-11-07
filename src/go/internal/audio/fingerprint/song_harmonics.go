package fingerprint

import "math"

// HarmonicPeak is one spectral peak (frequency + magnitude).
type HarmonicPeak struct{ Hz, Mag float64 }

// Just below A0 (27.5 Hz), so no real fundamental is lost while rumble and DC are ignored.
const harmonicMinHz = 25.0

// peaks[0] is the primary.
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

// Index 0 = unison, 7 = perfect fifth.
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

// Positive = sharp, against A4 = 440 equal temperament.
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
