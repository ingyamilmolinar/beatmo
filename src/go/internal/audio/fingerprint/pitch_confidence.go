package fingerprint

import (
	"math"
	"sort"
)

// DetectF0 always returns a number: on a hi-hat or clap it is an octave-guard artefact (56.5 Hz), and every
// descriptor assuming a harmonic series then reported a confident verdict about nothing. The per-frame YIN gate
// already knew; this keeps that judgement.

// Shared by the detector and the confidence score so the two cannot drift.
const frameConfidenceThreshold = 0.25

// In the middle of the empty band between the corpus's most periodic unpitched clip (0.42) and least periodic pitched
// one (0.94); `refs check -only f0` asserts the margin, so a change that narrows it fails rather than reclassifying.
const F0ReliableConfidence = 0.65

// F0Estimate is DetectF0's answer plus the evidence behind it.
type F0Estimate struct {
	// The SAME value DetectF0 returns: Confidence qualifies it, never replaces it, so an unreliable estimate stays
	// inspectable.
	Hz float64

	// How much of the signal YIN found periodic, scaled by how tightly those frames agreed.
	Confidence float64

	// Exposed so a surprising confidence can be explained rather than disbelieved.
	ConfidentFrames int
	TotalFrames     int
}

// Reliable reports whether Hz should be treated as a real fundamental.
func (e F0Estimate) Reliable() bool { return e.Hz > 0 && e.Confidence >= F0ReliableConfidence }

// The PRODUCT of energy coverage and agreement (IQR over median): a plucked note's quiet tail or one octave jump does
// not tank it, but a hit whose frames lock onto different resonances does.
func f0Estimate(hz float64, perFrameF0s []float64, totalFrames int, confidentEnergy, totalEnergy, fallbackDp float64) F0Estimate {
	e := F0Estimate{Hz: hz, ConfidentFrames: len(perFrameF0s), TotalFrames: totalFrames}
	if hz <= 0 {
		return e
	}

	// Under one frame there is no per-frame evidence, so the full-buffer run's aperiodicity decides.
	if totalFrames == 0 {
		e.TotalFrames = 1
		if fallbackDp < frameConfidenceThreshold {
			e.ConfidentFrames = 1
			e.Confidence = 1 - fallbackDp/frameConfidenceThreshold
		}
		return e
	}

	// The frame-count fallback guards a division by zero energy.
	coverage := float64(len(perFrameF0s)) / float64(totalFrames)
	if totalEnergy > 0 {
		coverage = confidentEnergy / totalEnergy
	}
	e.Confidence = coverage * f0Agreement(perFrameF0s)
	return e
}

// Falls to 0 as the IQR reaches the median: a spread as wide as the pitch is not one pitch.
func f0Agreement(f0s []float64) float64 {
	if len(f0s) < 2 {
		// One confident frame cannot disagree, so coverage alone carries the score: weak evidence among many, short audio
		// alone.
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

// Linear interpolation between neighbours.
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

// tau runs sr/F0SearchMaxHz .. sr/F0SearchMinHz. Exported because it CEILS what the detector can say: a fundamental
// below the floor pins at the floor lag WITH FULL CONFIDENCE (sub-bass kicks read exactly 50.00 Hz at 1.000), so
// confidence does not protect a caller; F0AtSearchFloor does.
const (
	F0SearchMinHz = 50
	F0SearchMaxHz = 2000
)

// Inside one semitone of the floor a true fundamental is indistinguishable from a pinned one, so it is unmeasurable;
// every instrument there declares a base frequency to fall back on.
const f0FloorGuardSemis = 1.0

// A caller needing an absolute pitch asks both: Reliable() asks whether the signal is periodic, which a floor-pinned
// sub-bass passes at 1.000.
func (e F0Estimate) F0AtSearchFloor() bool {
	return e.Hz > 0 && e.Hz < F0SearchMinHz*math.Pow(2, f0FloorGuardSemis/12)
}
