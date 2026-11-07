package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// A PITCH a listener would name, or a number that fits the waveform? A ride reads 501 Hz at confidence 0.998 with its
// lowest partial at 6.0× that; a real pitch, even a missing fundamental, has a partial in its first few harmonics.
// Inharmonicity cannot separate them (struck metal is genuinely inharmonic).

const (
	// Inside the empty band between the most extreme pitched instrument (3.08, the formant-heavy voices) and the first
	// unpitched one (6.00, ride); TestF0SupportBandIsCalibrated asserts both margins. A frequency with no partial in its
	// first four harmonics is not the fundamental of anything.
	f0SupportMaxPartialRatio = 4.0

	// -40 dB of the strongest bin: AnalyzePartialRatios filters peaks only relative to each other and peakPickPartials has
	// no floor, so in a band of noise ripple a "partial" lands inside the ratio bound by chance (7000 Hz on a 220 Hz sine
	// read as supported).
	f0SupportMinPartialRelAmp = 0.01

	// ~2.9 Hz bins at 48 kHz resolve the lowest instrument's first harmonics as separate peaks.
	f0SupportFFTSize = 16384
)

type F0Support struct {
	// ~1 when the fundamental carries energy, ~2–3 for a missing fundamental, far higher for a subharmonic artefact; 0 when
	// no significant partial was found.
	LowestPartialRatio float64

	// Carried for diagnosis; it does not feed Supported.
	SeriesDeviation float64

	Supported bool
}

// The SAME sustain window DetectF0Detailed uses: a struck attack is broadband noise, and MagnitudeSpectrum reads from
// the start of whatever it is handed. A zero value (unsupported) for f0 <= 0, silence or a segment too short.
func AnalyzeF0Support(w wave.Wave, f0 float64) F0Support {
	// Rejected EXPLICITLY: a NaN slips past `f0 <= 0` and reaches the right verdict only by accident of the arithmetic.
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

	// Ratios ascend, so this is the lowest; taking the minimum keeps it correct if that changes.
	lowest := pr.Ratios[0]
	for _, r := range pr.Ratios {
		if r > 0 && r < lowest {
			lowest = r
		}
	}

	// Real energy, not the tallest ripple in an empty band.
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

// THE call for naming a note: periodic at all (a hi-hat fails), inside the detector's range (a 44 Hz kick passes at
// 1.000) and present in the spectrum (a ride passes both). Comparing est.Hz to an expected frequency needs this.
func UsableAsPitch(w wave.Wave, est F0Estimate) (bool, F0Support) {
	sup := AnalyzeF0Support(w, est.Hz)
	return est.Reliable() && !est.F0AtSearchFloor() && sup.Supported, sup
}
