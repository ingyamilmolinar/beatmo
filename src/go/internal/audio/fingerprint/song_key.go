package fingerprint

import "math"

// Chroma folds the magnitude spectrum into 12 pitch classes (C..B), L1-normalized.
// Pitch class of bin freq f: round(12*log2(f/440)) mod 12, offset so index 0 = C.
func Chroma(mag []float64, binHz float64) [12]float64 {
	var ch [12]float64
	if binHz <= 0 {
		return ch
	}
	for k := 1; k < len(mag); k++ {
		f := float64(k) * binHz
		if f < 20 || f > 5000 {
			continue
		}
		midi := 69 + 12*math.Log2(f/440.0) // A4=440=MIDI69
		cIdx := ((int(math.Round(midi)) % 12) + 12) % 12
		ch[cIdx] += mag[k]
	}
	// L1 normalize.
	sum := 0.0
	for _, v := range ch {
		sum += v
	}
	if sum > 0 {
		for i := range ch {
			ch[i] /= sum
		}
	}
	return ch
}

// pearson returns the Pearson correlation of two 12-vectors.
func pearson(a, b [12]float64) float64 {
	var ma, mb float64
	for i := 0; i < 12; i++ {
		ma += a[i]
		mb += b[i]
	}
	ma /= 12
	mb /= 12
	var num, da, db float64
	for i := 0; i < 12; i++ {
		x, y := a[i]-ma, b[i]-mb
		num += x * y
		da += x * x
		db += y * y
	}
	if da == 0 || db == 0 {
		return 0
	}
	return num / math.Sqrt(da*db)
}

// DetectKey correlates chroma against all 24 rotated Krumhansl profiles.
func DetectKey(chroma [12]float64, cfg AnalysisConfig) (int, int, float64) {
	best := -2.0
	bestKey, bestMode := 0, 0
	for mode, profile := range [2][12]float64{cfg.KeyProfileMajor, cfg.KeyProfileMinor} {
		for key := 0; key < 12; key++ {
			var rot [12]float64
			for i := 0; i < 12; i++ {
				rot[i] = profile[((i-key)%12+12)%12]
			}
			if c := pearson(chroma, rot); c > best {
				best = c
				bestKey = key
				bestMode = mode
			}
		}
	}
	conf := (best + 1) / 2 // map [-1,1] → [0,1]
	return bestKey, bestMode, conf
}

// BestChromaShift returns the circular shift s (in semitones, -6..+5) of b that
// best correlates with a, and that correlation. Transposition-invariant compare.
func BestChromaShift(a, b [12]float64) (int, float64) {
	bestShift, bestCorr := 0, -2.0
	for s := 0; s < 12; s++ {
		var rot [12]float64
		for i := 0; i < 12; i++ {
			rot[i] = b[((i+s)%12+12)%12]
		}
		if c := pearson(a, rot); c > bestCorr {
			bestCorr = c
			bestShift = s
		}
	}
	// Normalize shift to -6..+5 for readability.
	if bestShift > 6 {
		bestShift -= 12
	}
	return bestShift, bestCorr
}
