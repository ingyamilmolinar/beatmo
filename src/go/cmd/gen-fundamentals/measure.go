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
	// SourceMeasured: a reliable DetectF0Detailed estimate over the rendered voice. Preferred where available: it is what
	// the instrument sounds, pitch envelope included.
	SourceMeasured Source = "measured"
	// SourceDeclared: the measurement was unreliable but the document declares a base-frequency param; true of the
	// noisier tuned drums (snare, kick-deep), where YIN cannot lock on but the head is tuned.
	SourceDeclared Source = "declared"
	// SourceVoiceFreq: only a positive voice_freq_hz, which on the modular kicks is the pitch-envelope TARGET rather than
	// the perceived pitch, so it ranks below a measurement.
	SourceVoiceFreq Source = "voice_freq_hz"
	// SourceMelodic: a melodic instrument (built to sound A3 at node pitch 0); its base is never measured, see measure().
	SourceMelodic Source = "melodic"
	// SourceNone: no fundamental from any source — genuinely unpitched noise
	// (hi-hats, claps, shaker, whisper). These get no note name.
	SourceNone Source = "none"
)

// 48 kHz because downsampling (YIN is quadratic in frame length) gives clean octave errors at full confidence. 1.5 s
// for evidence: a shorter window moves instruments near the confidence threshold to a different note name, so treat
// both as calibrated values, not tuning knobs.
const (
	renderSR  = 48000
	renderSec = 1.5
)

// Not an equality test: a declared fundamental is the oscillator's, and a pitch stage can bend the sounding pitch
// below it (or a physical kick sound a shell mode above it). It catches the OCTAVE-CLASS error (a harmonic, a
// waveguide's 2nd/3rd mode) at 12 or 19 semitones; 9 sits mid-way in the empty band drift_test.go asserts.
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

// declaredBaseHz reads the instrument's merged params, the values the render uses, preferring the base-frequency class
// over voice_freq_hz.
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

	// Believed only when periodic, inside the detector's range AND present in the spectrum (UsableAsPitch).
	trusted, sup := fingerprint.UsableAsPitch(w, est)
	m.FloorPinned = est.F0AtSearchFloor()
	m.LowestPartialRatio = sup.LowestPartialRatio
	m.Unsupported = est.Hz > 0 && !sup.Supported

	// A melodic instrument's base is never measured: node pitch 0 = A3 IS its declaration, and measurement only perturbs
	// it (YIN's octave artifact on struck strings puts the harpsichord at 440 against 220). Recorded as a source, not
	// omitted, so the decision stays visible.
	if audio.IsMelodicInstrument(id) {
		m.Source = SourceMelodic
		return m, nil
	}

	// Where both are meaningful they must not GROSSLY disagree (agreementToleranceSemis).
	if trusted && declIsBase && declHz > 0 {
		if d := semitonesBetween(est.Hz, declHz); d > agreementToleranceSemis {
			return m, fmt.Errorf(
				"%s: measured %.2f Hz (conf %.3f) contradicts declared %s=%.2f Hz by %.2f semitones "+
					"(tolerance %.2f) — one of them is wrong; investigate before regenerating",
				id, est.Hz, est.Confidence, declKey, declHz, d, agreementToleranceSemis)
		}
	}

	// A declared base-frequency param wins unless contradicted: exact, stable across DSP changes, and what
	// InstrumentPitchInfo already used. Measurement decides only where it is absent (modular kicks, congas, cowbell) or
	// wrong (the toms' pre-pitch-stage oscillator setting).
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

// A quarter tone: past it the two round to different note names, the only difference a user sees.
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
