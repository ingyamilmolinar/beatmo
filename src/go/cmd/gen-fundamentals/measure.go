package main

import (
	"fmt"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Source records where an instrument's base fundamental came from, so a
// surprising note name can be explained rather than merely disbelieved.
type Source string

const (
	// SourceMeasured: DetectF0Detailed returned a reliable estimate (confidence
	// >= fingerprint.F0ReliableConfidence) over the rendered voice. Preferred
	// whenever available: it is what the instrument actually sounds, including
	// the effect of a pitch envelope that no single declared param describes.
	SourceMeasured Source = "measured"
	// SourceDeclared: measurement was unreliable, but the recipe declares a
	// base-frequency param (fundamental / fm_base_freq / base_freq). True for
	// the noisier tuned drums (snare, kick-deep) where YIN cannot lock on but
	// the head is genuinely tuned.
	SourceDeclared Source = "declared"
	// SourceVoiceFreq: neither of the above, but the recipe carries a positive
	// voice_freq_hz. On the modular kicks this is the pitch-envelope TARGET
	// rather than the perceived pitch, so it ranks below a measurement.
	SourceVoiceFreq Source = "voice_freq_hz"
	// SourceMelodic: the recipe is in the curated melodic set, whose members
	// are all built to sound A3 at node pitch 0 and whose offsets already come
	// out right from their declared params. Their base is deliberately NOT
	// taken from a measurement — see measure() for why.
	SourceMelodic Source = "melodic"
	// SourceNone: no fundamental from any source — genuinely unpitched noise
	// (hi-hats, claps, shaker, whisper). These get no note name.
	SourceNone Source = "none"
)

// renderSR / renderSec are the analysis render settings.
//
// 48 kHz is not negotiable: YIN's cost is quadratic in frame length, so
// downsampling is the tempting optimisation, and it is wrong. At 24 kHz the
// violin reads 439.8 Hz and the cowbell 317 Hz — clean octave errors, at full
// confidence. The rate stays at the engine's own pre-render rate.
//
// 1.5 s is chosen for EVIDENCE, not speed. A 0.5 s window costs 23 ms against
// 147 ms, but this generator runs offline (the shipped binary reads the table,
// never this code), so the 120 ms buys nothing and the shorter window is not
// free: it reclassifies five instruments, and not harmlessly — tom-lofi's
// confidence falls under the threshold there, handing the note back to the
// declared `fundamental` 3.35 semitones away, a DIFFERENT note name.
//
// That sensitivity is a property of the signals, not a bug: an instrument
// whose classification moves with the window is one sitting near the
// confidence threshold (the cymbals, which have no stable period at all).
// Because the drift guard re-measures through this same constant, the two
// cannot disagree — but changing it WILL move note names, so treat it as a
// calibrated value rather than a tuning knob.
const (
	renderSR  = 48000
	renderSec = 1.5
)

// agreementToleranceSemis is how far a measured fundamental may sit from a
// declared base-frequency param before the generator treats the pair as a
// contradiction and fails.
//
// The two are NOT the same physical quantity, so this is deliberately not an
// equality test. A declared `fundamental` is the OSCILLATOR's base frequency;
// the measurement is what comes out after the post chain, and several recipes
// run a `pitch` stage that bends the sounding pitch below its oscillator
// setting (the toms drop 2.1-3.4 semitones this way). Demanding exact
// agreement would fail on correct data.
//
// What this guard is really for is the OCTAVE-CLASS error: a detector locking
// onto a harmonic or a waveguide settling on its second or third mode (the
// 2026-09-12 bowed-string fault put the cello a TWELFTH high). Those land at
// 12 or 19 semitones.
//
// Calibrated on the roster: the largest legitimate disagreement was tom-lofi at
// 3.35 semitones until the 2026-10-02 PHYSICAL kicks (variant 8), whose
// sounding pitch is a membrane/shell mode above the declared fundamental:
// kick-airy measures its 1.38·f0 shell ring at 5.83 semitones, kick-brutal /
// kick-wood their 1.72·f0 membrane mode at ~5.7. The smallest octave-class error
// is 12, so 9 sits in the middle of the 6.17-semitone empty band with >= 2.17
// margin on both sides. drift_test.go asserts that band, so a DSP change that
// narrows it fails rather than silently reclassifying an instrument.
const agreementToleranceSemis = 9.0

// Measurement is one instrument's resolved base fundamental plus the evidence.
type Measurement struct {
	ID         instrument.ID
	BaseHz     float64
	Source     Source
	MeasuredHz float64
	Confidence float64
	DeclaredHz float64
	// DeclaredKey is the param name DeclaredHz came from, "" when none.
	DeclaredKey string
	// FloorPinned records that the measurement sat at the detector's lower
	// search bound and was rejected for it, even at high confidence.
	FloorPinned bool
	// LowestPartialRatio is where the spectrum's lowest significant partial
	// sits relative to the measured f0. Far above 1 means the claimed
	// fundamental carries no energy at all — a cymbal, not a pitch.
	LowestPartialRatio float64
	// Unsupported records that the measurement was rejected because the
	// spectrum does not contain the frequency it claims.
	Unsupported bool
}

// declaredBaseHz returns the instrument's declared base frequency and the param
// it came from, preferring the base-frequency class over voice_freq_hz.
//
// It reads the MERGED params (recipe defaults with per-instrument overrides on
// top), the same resolution the render path uses, so a frequency baked into a
// seed counts exactly as a user knob would.
func declaredBaseHz(id instrument.ID) (hz float64, key string, isBaseClass bool) {
	p := audio.MergedInstrumentParamsRO(id)
	for _, k := range audio.BaseFrequencyParamNames() {
		if v, ok := p[k]; ok && v > 0 {
			return v, k, true
		}
	}
	if v, ok := p["voice_freq_hz"]; ok && v > 0 {
		return v, "voice_freq_hz", false
	}
	return 0, "", false
}

// measure resolves one instrument's base fundamental under the precedence in
// the Source docs, and reports a measured/declared contradiction as an error.
func measure(id instrument.ID) (Measurement, error) {
	m := Measurement{ID: id, Source: SourceNone}
	declHz, declKey, declIsBase := declaredBaseHz(id)
	m.DeclaredHz, m.DeclaredKey = declHz, declKey

	w, err := synthmatch.RenderInstrument(id, 0, renderSR, renderSec)
	if err != nil {
		return m, fmt.Errorf("render %s: %w", id, err)
	}
	est := fingerprint.DetectF0Detailed(w)
	m.MeasuredHz, m.Confidence = est.Hz, est.Confidence

	// A measurement must clear all three gates to be believed as an absolute
	// frequency — periodic enough, inside the detector's range, and actually
	// present in the spectrum. UsableAsPitch is that conjunction; its doc
	// explains why none of the three is redundant with the others.
	trusted, sup := fingerprint.UsableAsPitch(w, est)
	m.FloorPinned = est.F0AtSearchFloor()
	m.LowestPartialRatio = sup.LowestPartialRatio
	m.Unsupported = est.Hz > 0 && !sup.Supported

	// A melodic instrument's base is never measured. The curated melodic set is
	// built so that node pitch 0 sounds A3, and that convention IS the
	// declaration even where no base-frequency param spells it out — the
	// existing offsets are already correct, and measurement can only perturb
	// them. It does, too: the harpsichord measures 440 Hz against its true 220
	// (the octave artifact YIN is known for on plucked/struck strings, the same
	// failure that put guitar and organ an octave out), and kalimba lands 2.37
	// semitones off. Both are errors; both would have shipped as wrong note
	// names on instruments that display correctly today.
	//
	// So the table's job for these is to say "nothing to add" and let
	// InstrumentPitchInfo keep its declared-parameter answer. Recording it as a
	// source rather than omitting the row keeps the decision visible.
	if audio.IsMelodicInstrument(id) {
		m.Source = SourceMelodic
		return m, nil
	}

	// Cross-check before choosing: where both sources are meaningful they must
	// not GROSSLY disagree, so the pair validates rather than merely offering
	// two guesses. See agreementToleranceSemis for what "grossly" means.
	if trusted && declIsBase && declHz > 0 {
		if d := semitonesBetween(est.Hz, declHz); d > agreementToleranceSemis {
			return m, fmt.Errorf(
				"%s: measured %.2f Hz (conf %.3f) contradicts declared %s=%.2f Hz by %.2f semitones "+
					"(tolerance %.2f) — one of them is wrong; investigate before regenerating",
				id, est.Hz, est.Confidence, declKey, declHz, d, agreementToleranceSemis)
		}
	}

	// Precedence. A DECLARED base-frequency param wins whenever it is not
	// contradicted, even though a measurement is available: it is exact, it is
	// stable across DSP changes, and it is what InstrumentPitchInfo already
	// used, so preferring it keeps every currently-pitched instrument's offset
	// bit-identical and confines this change to the instruments that actually
	// needed fixing. Measurement is the authority only where the declaration is
	// absent (the modular kicks, congas, cowbell) or demonstrably wrong (the
	// toms, whose `fundamental` is the pre-pitch-stage oscillator setting).
	switch {
	case declIsBase && (!trusted || agreesWithDeclared(est.Hz, declHz)):
		m.BaseHz, m.Source = declHz, SourceDeclared
	case trusted:
		m.BaseHz, m.Source = est.Hz, SourceMeasured
	case declHz > 0:
		m.BaseHz, m.Source = declHz, SourceVoiceFreq
	}
	return m, nil
}

// agreesWithDeclared reports whether a measurement is close enough to a
// declared base frequency that the declared one can stand in for it.
//
// The bar is a quarter tone: past that the two round to different note names,
// which is the only difference a user can actually see.
func agreesWithDeclared(measuredHz, declHz float64) bool {
	if measuredHz <= 0 || declHz <= 0 {
		return false
	}
	return semitonesBetween(measuredHz, declHz) <= 0.5
}

// semitonesBetween is the absolute interval between two frequencies, or 0 when
// either is absent.
func semitonesBetween(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	return math.Abs(12 * math.Log2(a/b))
}
