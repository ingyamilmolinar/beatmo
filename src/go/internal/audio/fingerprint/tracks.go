package fingerprint

import "math"

// Fingerprint looks at one mid-note second, and a note's IDENTITY often lives in its time evolution (a trumpet's
// crescendo, its harmonics blooming, a noisy attack that cleans up). Tracks measures each quantity PER FRAME across the
// whole signal, and TrackStats reduces any of them uniformly.

// ~46 ms frames every ~11.6 ms: resolves an attack and a 30 Hz warble (frame-rate Nyquist ~43 Hz), meaningful down to a
// ~50 Hz fundamental. Matches computeTemporal's framing.
const (
	trackFFTSize = 2048
	trackHop     = 512
)

// 16, like Fingerprint.Partials, so both views describe the same ladder.
const trackHarmonicCount = 16

// Equal lengths and one HopSec, so any two can be correlated directly.
type Tracks struct {
	HopSec    float64
	NumFrames int

	RMS      []float64 // per-frame RMS of the time-domain samples
	Crest    []float64 // per-frame peak/RMS — transient sharpness over time
	Centroid []float64 // spectral centroid (Hz)
	Rolloff  []float64 // 85% spectral rolloff (Hz)
	Flatness []float64 // spectral flatness (0 = tonal, 1 = noise)

	// Zero throughout without an F0; the descriptors reading it are then gated off, never a fabricated 0.
	NoiseFraction []float64

	// dBFS: do the upper harmonics BLOOM over the note, the trumpet's decisive cue.
	Harmonics [trackHarmonicCount][]float64

	// From the bundle's Fingerprint, so the battery quotes one pitch; 0 when unpitched.
	F0Hz float64
}

// Tracks returns the frame-wise analysis of the whole signal, computed once.
func (b *Bundle) Tracks() *Tracks {
	b.tracksOnce.Do(func() { b.tracks = computeTracks(b) })
	return b.tracks
}

func computeTracks(b *Bundle) *Tracks {
	st := b.STFT(trackFFTSize, trackHop)
	f0 := b.Fingerprint().F0Hz

	t := &Tracks{
		HopSec:    st.HopSec,
		NumFrames: len(st.Frames),
		F0Hz:      f0,
	}
	if t.NumFrames == 0 {
		return t
	}

	t.RMS = make([]float64, t.NumFrames)
	t.Crest = make([]float64, t.NumFrames)
	t.Centroid = make([]float64, t.NumFrames)
	t.Rolloff = make([]float64, t.NumFrames)
	t.Flatness = make([]float64, t.NumFrames)
	t.NoiseFraction = make([]float64, t.NumFrames)
	for k := range t.Harmonics {
		t.Harmonics[k] = make([]float64, t.NumFrames)
	}

	samples := b.Wave().Samples
	for i, mag := range st.Frames {
		// --- time-domain: RMS and crest over this frame's samples ----------
		start := i * trackHop
		end := start + trackFFTSize
		if end > len(samples) {
			end = len(samples)
		}
		var sumSq, peak float64
		for _, v := range samples[start:end] {
			sumSq += v * v
			if a := math.Abs(v); a > peak {
				peak = a
			}
		}
		n := float64(end - start)
		if n > 0 {
			t.RMS[i] = math.Sqrt(sumSq / n)
		}
		if t.RMS[i] > 1e-12 {
			t.Crest[i] = peak / t.RMS[i]
		}

		// --- spectral shape, reusing the shared helpers --------------------
		t.Centroid[i] = spectralCentroidHz(mag, st.BinHz)
		t.Flatness[i] = spectralFlatnessOf(mag)
		t.Rolloff[i] = rolloffHz(mag, st.BinHz, 0.85)

		// --- harmonic ladder + noise fraction ------------------------------
		if f0 > 0 {
			t.NoiseFraction[i] = frameNoiseFraction(mag, st.BinHz, f0)
			for k := 0; k < trackHarmonicCount; k++ {
				t.Harmonics[k][i] = ampToDBFloor(harmonicPeak(mag, st.BinHz, f0*float64(k+1)))
			}
		}
	}
	return t
}

// Energy, DC skipped, exactly as computeSpectral's 85 % rolloff, so track and scalar measure the same thing.
func rolloffHz(mag []float64, binHz, frac float64) float64 {
	total := 0.0
	for i := 1; i < len(mag); i++ {
		total += mag[i] * mag[i]
	}
	if total <= 1e-24 {
		return 0
	}
	cum := 0.0
	for i := 1; i < len(mag); i++ {
		cum += mag[i] * mag[i]
		if cum >= frac*total {
			return float64(i) * binHz
		}
	}
	return float64(len(mag)-1) * binHz
}

// Assumes partials on a harmonic series, so a bell's tonal energy counts as noise; readers are gated on a pitched
// signal, and the report warns when the reference's series deviation is high.
func frameNoiseFraction(mag []float64, binHz, f0 float64) float64 {
	total := 0.0
	for i := 1; i < len(mag); i++ {
		total += mag[i] * mag[i]
	}
	if total <= 1e-24 {
		return 0
	}
	harmonic := 0.0
	for k := 1; k <= trackHarmonicCount; k++ {
		center := int(f0*float64(k)/binHz + 0.5)
		for d := -1; d <= 1; d++ {
			if bin := center + d; bin >= 0 && bin < len(mag) {
				harmonic += mag[bin] * mag[bin]
			}
		}
	}
	nf := 1 - harmonic/total
	if nf < 0 {
		return 0
	}
	return nf
}

// The window widens with frequency (1.3 % per harmonic): a fixed ±1 bin misses a vibrato-smeared upper partial.
func harmonicPeak(mag []float64, binHz, targetHz float64) float64 {
	if targetHz <= 0 || binHz <= 0 {
		return 0
	}
	halfWidthHz := math.Max(binHz, 0.013*targetHz)
	lo := int((targetHz - halfWidthHz) / binHz)
	hi := int((targetHz+halfWidthHz)/binHz) + 1
	if lo < 0 {
		lo = 0
	}
	if hi >= len(mag) {
		hi = len(mag) - 1
	}
	best := 0.0
	for i := lo; i <= hi; i++ {
		if mag[i] > best {
			best = mag[i]
		}
	}
	return best
}

// A hard floor, not -Inf, keeps every statistic finite.
const dbFloor = -120.0

func ampToDBFloor(a float64) float64 {
	if a <= 1e-12 {
		return dbFloor
	}
	db := 20 * math.Log10(a)
	if db < dbFloor {
		return dbFloor
	}
	return db
}
