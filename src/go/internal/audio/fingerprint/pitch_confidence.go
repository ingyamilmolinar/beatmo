package fingerprint

import (
	"math"
	"sort"
)

// pitch_confidence.go — how much to trust DetectF0's answer.
//
// DetectF0 always returns a number. For a hi-hat, a clap or a snare that
// number is whatever the octave guard settled on over a spectrum with no
// harmonic series in it at all — the corpus's `hihat-vcsl` reads 56.5 Hz,
// which is not a pitch, it is an artifact. Every descriptor built on an
// ASSUMED harmonic series (noise fraction's +/-1-bin notches, the partial
// ratios, inharmonicity, the per-harmonic amplitude tracks) is measuring
// nothing on those signals, and reported a confident verdict anyway: the
// listener called the synth hi-hat "too tonal" while noise.fraction claimed
// the synth had MORE noise than the reference (0.9995 vs 0.9187).
//
// The detector already knew. Its per-frame YIN loop gates every frame on
// aperiodicity (frameConfidenceThreshold), and a bare float64 return threw
// that judgement away. This file gives it back.

// frameConfidenceThreshold is the per-frame YIN aperiodicity ceiling: a frame
// whose d'(tau) minimum sits at or above this is too aperiodic for its period
// estimate to be trusted, and is excluded from the median. Shared by the
// detector and the confidence score so the two cannot drift apart.
const frameConfidenceThreshold = 0.25

// F0ReliableConfidence is the confidence at or above which a detected
// fundamental is treated as a real pitch rather than an artifact of the
// octave guard.
//
// Calibrated on the 60-clip reference corpus (re-checked by `refs check -only
// f0`, cmd/refs/check_f0.go): the highest-scoring unpitched
// percussion clip reads 0.420 and the lowest-scoring unambiguously pitched
// clip reads 0.944, so 0.65 sits near the middle of a 0.52-wide empty band
// with >= 0.23 margin on both sides. That check asserts the margin, so a
// detector change that narrows the gap fails it rather than silently
// reclassifying a clip.
const F0ReliableConfidence = 0.65

// F0Estimate is DetectF0's answer plus the evidence behind it.
type F0Estimate struct {
	// Hz is the detected fundamental, or 0 when none was found. It carries
	// the SAME value DetectF0 returns — Confidence qualifies it, never
	// replaces it, so an unreliable estimate can still be reported and
	// inspected rather than silently zeroed.
	Hz float64

	// Confidence is 0..1: how much of the signal YIN found periodic, scaled
	// by how tightly those periodic frames agreed on one period.
	Confidence float64

	// ConfidentFrames / TotalFrames are the raw counts the score is built
	// from, exposed so a surprising confidence can be explained rather than
	// merely disbelieved.
	ConfidentFrames int
	TotalFrames     int
}

// Reliable reports whether Hz should be treated as a real fundamental.
func (e F0Estimate) Reliable() bool { return e.Hz > 0 && e.Confidence >= F0ReliableConfidence }

// f0Estimate scores a detection.
//
// Two independent things have to hold for a fundamental to be real, and the
// score is their PRODUCT because either one failing is disqualifying on its
// own:
//
//   - COVERAGE: what share of the signal's ENERGY sits in frames YIN found
//     periodic enough to trust. Noise fails here — almost no frame passes the
//     aperiodicity gate. Energy-weighted rather than frame-counted because a
//     plucked or struck note (koto, felt piano, harp) spends most of its
//     FRAMES in a decay tail near the noise floor, where YIN correctly gives
//     up; counting those equally with the loud periodic attack scored real
//     pitched instruments below unpitched percussion.
//   - AGREEMENT: how tightly the trusted frames settled on one period,
//     measured as the interquartile spread relative to the median. A signal
//     whose "pitch" jumps an octave frame to frame fails here even when every
//     frame individually looked periodic — which is the percussive case, where
//     each frame locks onto a different transient resonance.
//
// IQR rather than standard deviation: one octave-jumped frame in an otherwise
// steady note should not tank the score, and vibrato (a smooth excursion) must
// not read as disagreement.
func f0Estimate(hz float64, perFrameF0s []float64, totalFrames int, confidentEnergy, totalEnergy, fallbackDp float64) F0Estimate {
	e := F0Estimate{Hz: hz, ConfidentFrames: len(perFrameF0s), TotalFrames: totalFrames}
	if hz <= 0 {
		return e
	}

	// Fewer than one full frame of audio: no per-frame evidence exists, so
	// fall back to the single full-buffer run's own aperiodicity. Short test
	// signals and very short clips take this path.
	if totalFrames == 0 {
		e.TotalFrames = 1
		if fallbackDp < frameConfidenceThreshold {
			e.ConfidentFrames = 1
			e.Confidence = 1 - fallbackDp/frameConfidenceThreshold
		}
		return e
	}

	// Energy-weighted coverage, with a frame-count fallback for a signal
	// carrying no energy at all (which cannot happen alongside hz>0 in
	// practice, but must not divide by zero if it ever does).
	coverage := float64(len(perFrameF0s)) / float64(totalFrames)
	if totalEnergy > 0 {
		coverage = confidentEnergy / totalEnergy
	}
	e.Confidence = coverage * f0Agreement(perFrameF0s)
	return e
}

// f0Agreement returns 1 for per-frame estimates that agree exactly and falls
// to 0 as their interquartile spread reaches the median itself (an IQR as wide
// as the pitch means the "pitch" is not one pitch).
func f0Agreement(f0s []float64) float64 {
	if len(f0s) < 2 {
		// A single confident frame cannot disagree with anything. Coverage
		// alone then carries the score, which is the honest reading: one
		// periodic frame out of many is weak evidence, and out of one is
		// simply short audio.
		return 1
	}
	sorted := append([]float64(nil), f0s...)
	sort.Float64s(sorted)
	median := percentileSorted(sorted, 0.5)
	if median <= 0 {
		return 0
	}
	spread := (percentileSorted(sorted, 0.75) - percentileSorted(sorted, 0.25)) / median
	return math.Max(0, 1-spread)
}

// percentileSorted reads the p-quantile (0..1) of an ascending slice by linear
// interpolation between neighbours.
func percentileSorted(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	return sorted[lo] + (pos-float64(lo))*(sorted[hi]-sorted[lo])
}

// F0SearchMinHz / F0SearchMaxHz bound the YIN lag search in yinF0ForFrame:
// tau runs over sr/F0SearchMaxHz .. sr/F0SearchMinHz.
//
// They are exported because the bound is not merely an implementation detail —
// it is a CEILING ON WHAT THE DETECTOR CAN SAY. A signal whose true fundamental
// lies below F0SearchMinHz cannot be reported as such; the search pins at the
// floor lag and, because that lag genuinely minimises d'(tau), it does so with
// FULL confidence. Three shipped instruments sit in that blind spot (the 808
// kick at 44 Hz, the dry kick at 45 Hz, the sub-bass at 45 Hz) and every one of
// them measured exactly 50.00 Hz at confidence 1.000 — a confidently wrong
// answer, not a detectable failure.
//
// Confidence therefore does NOT protect a caller from the floor. Any caller
// that treats a detected f0 as an absolute frequency must additionally reject
// estimates sitting within a small margin of F0SearchMinHz; F0AtSearchFloor
// below is that test.
const (
	F0SearchMinHz = 50
	F0SearchMaxHz = 2000
)

// f0FloorGuardSemis is how far above F0SearchMinHz an estimate must sit before
// it is believed. One semitone: a true fundamental inside that band is
// indistinguishable from a pinned one, so the band is treated as unmeasurable
// rather than guessed at. Erring this way is safe — every instrument currently
// in the band declares a base frequency to fall back on.
const f0FloorGuardSemis = 1.0

// F0AtSearchFloor reports whether an estimate is too close to the detector's
// lower search bound to be trusted as an absolute frequency.
//
// Reliable() and F0AtSearchFloor answer DIFFERENT questions and a caller
// needing an absolute pitch must ask both: Reliable() asks "is this signal
// periodic at all", which a floor-pinned sub-bass passes with confidence 1.000.
func (e F0Estimate) F0AtSearchFloor() bool {
	return e.Hz > 0 && e.Hz < F0SearchMinHz*math.Pow(2, f0FloorGuardSemis/12)
}
