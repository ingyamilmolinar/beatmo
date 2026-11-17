package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// pitch_support.go — does the SPECTRUM actually back up the detected f0?
//
// DetectF0Detailed answers "is this signal periodic, and at what period".
// Confidence qualifies that answer honestly, and F0AtSearchFloor catches the
// one place the detector cannot see. Neither asks the separate question this
// file asks: is the frequency it returned a PITCH — something a listener would
// name a note — or merely a number that fits the waveform?
//
// A cymbal is the case that forces the distinction. A ride is a chaotic
// inharmonic plate with no perceived pitch at all, yet it reads 501 Hz at
// confidence 0.998: its dense mode cluster is periodic enough for YIN, and the
// octave guard settles on a subharmonic well below any of the actual modes. The
// tell is in the spectrum: the ride's LOWEST significant partial sits at 6.0x
// the reported f0, the crash's at 9.7x, a hi-hat's at 14.6x and a clap's at
// 23.9x. There is no energy whatsoever at the frequency each one claims to
// sound. A real pitch — even one whose fundamental is missing and reconstructed
// by the ear, as a warm cello's is — always shows a partial within the first
// few harmonics.
//
// So the gate is fundamental SUPPORT: the lowest significant partial must sit
// within the first few harmonics of the claimed f0.
//
// Why not SeriesDeviation, the obvious candidate: measured across the 133
// instrument roster it does NOT separate these populations safely. The worst
// legitimately-pitched instrument (glass-wash, 1.561) and the first cymbal
// (ride, 1.933) are 1.24x apart, with glockenspiel at 1.384 and celesta at
// 0.788 crowding the same region — struck metal is genuinely inharmonic, so an
// inharmonicity axis cannot help but confuse it with a cymbal. Fundamental
// support separates the same two populations by 1.95x (see
// f0SupportMaxPartialRatio). SeriesDeviation is still reported here because it
// is the right axis for the string-vs-bar question it was built for; it is just
// not the right one for pitched-vs-unpitched.

const (
	// f0SupportMaxPartialRatio is the highest harmonic number at which the
	// lowest significant partial may sit for the f0 to be considered
	// spectrally supported.
	//
	// Calibrated on the 133-instrument roster, measured over the same sustain
	// window the verdict uses: the highest lowest-partial ratio among
	// unambiguously pitched instruments is 3.08 (voice-opera and voice-ahh,
	// whose fundamental is weak against their formants), and the lowest among
	// the unpitched is 6.00 (ride, then crash 9.68, hi-hats 14.61-21.37, claps
	// 23.86). 4.0 sits inside that 1.95x empty band with 4.5 semitones of
	// margin below and 7.0 above. TestF0SupportBandIsCalibrated asserts both
	// margins, so a DSP change that narrows the band fails rather than silently
	// reclassifying an instrument.
	//
	// Physically it reads as: a frequency with no partial inside its first
	// four harmonics is not the fundamental of anything.
	f0SupportMaxPartialRatio = 4.0

	// f0SupportMinPartialRelAmp is the floor the lowest partial's magnitude must
	// clear, as a fraction of the spectrum's strongest bin (0.01 = -40 dB).
	//
	// Without it the support measure can be fooled into endorsing a fabricated
	// f0. AnalyzePartialRatios filters candidate peaks RELATIVE TO EACH OTHER
	// (partialRatioMinRelAmp, against the strongest peak it found), and
	// peakPickPartials imposes no absolute floor at all — so in a band holding
	// nothing but noise-floor ripple, that ripple is promoted to "partials" and
	// one of them lands inside the ratio bound by chance. Asking support for
	// 7000 Hz on a clean 220 Hz sine returned a lowest partial of 1.52x and a
	// verdict of SUPPORTED before this floor existed.
	//
	// The roster never exercised that path, because every f0 it asks about came
	// from real periodicity. A caller passing an f0 from somewhere else — a
	// score, a user, a different detector — would have.
	f0SupportMinPartialRelAmp = 0.01

	// f0SupportFFTSize is the analysis FFT length. At 48 kHz it gives ~2.9 Hz
	// bins — fine enough to resolve the first harmonics of the lowest
	// instrument on the roster (the 42 Hz deep kick) as separate peaks.
	f0SupportFFTSize = 16384
)

// F0Support reports whether a detected f0 is backed by the spectrum, and the
// evidence behind the verdict.
type F0Support struct {
	// LowestPartialRatio is the lowest significant partial's frequency divided
	// by the candidate f0 — ~1 when the fundamental itself carries energy, ~2
	// or ~3 for a missing-fundamental tone, and far higher when the candidate
	// is a subharmonic artifact with nothing under it. 0 when no significant
	// partial was found.
	LowestPartialRatio float64

	// SeriesDeviation is AnalyzePartialRatios' string-vs-bar scalar, carried
	// through for diagnosis. It does NOT feed Supported — see the file doc.
	SeriesDeviation float64

	// Supported is the verdict: the spectrum has a partial close enough to the
	// candidate to call it a fundamental.
	Supported bool
}

// AnalyzeF0Support measures whether f0 is spectrally supported in w.
//
// It analyses the SAME sustain window DetectF0Detailed does, deliberately: the
// attack of a struck instrument is broadband noise, so measuring support over a
// window the f0 was not derived from would judge a different signal than the
// one that produced the number. (wave.MagnitudeSpectrum reads from the start of
// whatever it is handed, so this has to be arranged rather than assumed.)
//
// Returns a zero value — Supported false — for f0 <= 0, silence, or a segment
// too short to transform, so an unusable input can never read as supported.
func AnalyzeF0Support(w wave.Wave, f0 float64) F0Support {
	// NaN and Inf are rejected EXPLICITLY rather than left to fall out of the
	// comparisons below. A NaN f0 does slip past `f0 <= 0` (every comparison
	// with NaN is false) and happens to reach an unsupported verdict anyway,
	// via ratios that are themselves NaN — correct by accident, and only while
	// the arithmetic downstream keeps its current shape.
	if f0 <= 0 || math.IsNaN(f0) || math.IsInf(f0, 0) || len(w.Samples) == 0 {
		return F0Support{}
	}
	seg := sustainWindow(w)
	if len(seg.Samples) < 4 {
		return F0Support{}
	}
	mag, binHz := wave.MagnitudeSpectrum(seg, f0SupportFFTSize, wave.WindowHann)
	if binHz <= 0 {
		return F0Support{}
	}
	pr := AnalyzePartialRatios(mag, binHz, f0)
	if len(pr.Ratios) == 0 {
		return F0Support{}
	}

	// Ratios ascend by frequency, so the first entry is the lowest partial.
	// Taking the minimum anyway keeps this correct if that ordering ever
	// changes, at no cost.
	lowest := pr.Ratios[0]
	for _, r := range pr.Ratios {
		if r > 0 && r < lowest {
			lowest = r
		}
	}

	// The lowest partial must carry real energy, not merely be the tallest
	// ripple in an empty band — see f0SupportMinPartialRelAmp.
	peak := 0.0
	for _, m := range mag {
		if m > peak {
			peak = m
		}
	}
	hasEnergy := peak > 0 &&
		k1Magnitude(mag, binHz, f0*lowest) >= f0SupportMinPartialRelAmp*peak

	return F0Support{
		LowestPartialRatio: lowest,
		SeriesDeviation:    pr.SeriesDeviation,
		Supported:          lowest > 0 && lowest <= f0SupportMaxPartialRatio && hasEnergy,
	}
}

// UsableAsPitch reports whether est may be used as an ABSOLUTE pitch for w —
// the single call a caller that wants to NAME A NOTE should make.
//
// All three gates are necessary and each catches a failure the others cannot:
//
//   - Reliable(): is the signal periodic at all. A hi-hat fails here.
//   - F0AtSearchFloor(): is the answer inside the range the detector can see.
//     A 44 Hz kick pins at the 50 Hz floor and passes Reliable at confidence
//     1.000, so this gate is not redundant with it.
//   - AnalyzeF0Support(): does the spectrum contain the claimed fundamental. A
//     ride cymbal passes BOTH gates above at confidence 0.998 and is still not
//     a pitch.
//
// Not every f0 consumer needs this. There are two kinds, and only one does:
//
//   - ABSOLUTE consumers compare the detected frequency against a nominal one —
//     naming a note, checking an instrument is in tune, building the
//     fundamentals table. A wrong number is a wrong answer, so they call this.
//   - ANCHOR consumers use f0 as the reference the harmonic descriptors are
//     measured RELATIVE to (partial ratios, inharmonicity, harmonic tracks).
//     They are already served by F0Confidence, which is what the descriptor
//     registry suppresses harmonic axes on.
//
// If you are about to write `hz := est.Hz` and then compare it to a frequency
// you expect, you are the first kind.
func UsableAsPitch(w wave.Wave, est F0Estimate) (bool, F0Support) {
	sup := AnalyzeF0Support(w, est.Hz)
	return est.Reliable() && !est.F0AtSearchFloor() && sup.Supported, sup
}
