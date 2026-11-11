package fingerprint

import "math"

// tracks.go — frame-wise measurement of the WHOLE signal.
//
// THE PROBLEM THIS FIXES
// ----------------------
// Fingerprint measures spectral shape, MFCC, partials, formants, vibrato,
// coupling and noise over a SINGLE window: skip SustainSkipSec (0.3 s), take
// up to SustainLenSec (1.3 s). One second, from the middle of the note. Only
// computeTemporal and MacroCentroidRMSCorr see the whole signal at all.
//
// AGENTS.synth.md records the cost of that in its own words: "Analyse the
// WHOLE signal over TIME, not a single steady window (the trumpet
// breakthrough). A note's IDENTITY often lives in its time-evolution, which a
// mid-sustain FFT throws away." The real trumpet is defined by a 500 ms
// crescendo, harmonics that bloom progressively brighter as it swells, and a
// noisy attack that cleans up -- none of which a mid-sustain snapshot can
// see. Four separate tuning attempts failed before that was measured.
//
// Tracks computes each of those quantities PER FRAME across the entire
// signal. TrackStats (trackstats.go) then reduces any track with one uniform
// statistic set, so "the whole-signal statistics" is a mechanical
// cross-product rather than a pile of bespoke scalars.

// trackFFTSize and trackHop define the analysis framing. 2048 at 44.1 kHz is
// ~46 ms per frame with ~11.6 ms between frames: fine enough to resolve an
// attack transient and a 30 Hz amplitude warble (the Nyquist of the frame
// rate is ~43 Hz), coarse enough that the per-frame spectrum is meaningful
// down to a ~50 Hz fundamental. It matches computeTemporal's existing framing
// so the two agree.
const (
	trackFFTSize = 2048
	trackHop     = 512
)

// trackHarmonicCount is how many harmonics get their own amplitude track.
// Matches Fingerprint.Partials' 16 so the per-frame view and the
// single-window view describe the same ladder.
const trackHarmonicCount = 16

// Tracks holds one value per STFT frame for every frame-wise quantity the
// battery measures. Every slice has the same length (NumFrames) and shares
// HopSec, so any two can be correlated directly.
type Tracks struct {
	HopSec    float64
	NumFrames int

	RMS      []float64 // per-frame RMS of the time-domain samples
	Crest    []float64 // per-frame peak/RMS — transient sharpness over time
	Centroid []float64 // spectral centroid (Hz)
	Rolloff  []float64 // 85% spectral rolloff (Hz)
	Flatness []float64 // spectral flatness (0 = tonal, 1 = noise)

	// NoiseFraction is the per-frame analogue of Fingerprint.NoiseRatio: the
	// share of energy NOT within +/-1 bin of an assumed harmonic of F0. Zero
	// throughout when no F0 was detected, in which case the descriptors that
	// read it are gated off rather than reporting a fabricated 0.
	NoiseFraction []float64

	// Harmonics[k] is the amplitude track of the (k+1)-th harmonic in dB
	// relative to full scale. This is the "do the upper harmonics BLOOM in
	// over the note" measurement -- the trumpet's decisive cue.
	Harmonics [trackHarmonicCount][]float64

	// F0Hz is the fundamental used for the harmonic tracks, taken from the
	// bundle's Fingerprint so the whole battery quotes one pitch. 0 when
	// unpitched.
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

// rolloffHz returns the frequency below which `frac` of the spectral ENERGY
// lies. Mirrors computeSpectral's 85% rolloff exactly (energy, i.e. squared
// magnitude, skipping DC) so the per-frame track and the single-window scalar
// measure the same thing.
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

// frameNoiseFraction is the per-frame analogue of Fingerprint.NoiseRatio:
// 1 - (energy within +/-1 bin of each k*f0) / (total energy).
//
// CAVEAT, inherited deliberately rather than papered over: this assumes the
// signal's real partials sit on a harmonic series. For a bell or a tuned bar
// they do not, and genuinely tonal energy is then counted as noise. The
// descriptors that read this track are gated on a pitched signal, and the
// report carries an explicit caveat when the reference's own partial-series
// deviation is high.
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

// harmonicPeak returns the largest magnitude within a small window around
// targetHz. The window widens with frequency (a fixed +/-1 bin misses a
// vibrato-smeared upper partial) using the same 1.3%-per-harmonic tolerance
// computeSpectral applies.
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

// dbFloor is the level reported for a partial that is effectively absent.
// A hard floor (rather than -Inf) keeps every downstream statistic finite;
// -120 dB is far below anything audible in a peak-normalized signal.
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
