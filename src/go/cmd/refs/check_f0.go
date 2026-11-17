//go:build !js

package main

import (
	"fmt"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// The f0 checks — the calibration behind fingerprint.F0ReliableConfidence,
// run against the reference corpus rather than against synthetic tones.
//
// The threshold gates every descriptor that assumes a harmonic series
// (noise.fraction, the partial.* family, the per-harmonic tracks). Picking it
// from synthetic signals would prove nothing: a synthetic tone scores 1.000
// and synthetic noise scores 0.000 whatever the threshold is. What matters is
// that REAL unpitched percussion and REAL pitched instruments separate, with
// the threshold in the empty space between the two populations.

// unpitchedRefs are corpus clips with no fundamental to find: the harmonic-
// series assumption is simply false for them, whatever number the octave
// guard returns. Every id must be catalogued (TestF0CheckTablesNameCataloguedRefs):
// a clip that is not fetched here is skipped, so a REMOVED one would vanish
// from the check silently -- ride-vcsl did when ride-local replaced it.
var unpitchedRefs = []string{
	"hihat-vcsl", "shaker-vcsl", "crash-vcsl", "cowbell-vcsl",
	"rimshot-vcsl", "snare-vcsl", "snare-freesound-alt",
	"zgump-kick-freesound", "dnb-kick-freesound", "kick-acoustic-vcsl",
	"kick-acoustic-freesound-alt1", "clap-vcsl", "sidestick-vcsl",
}

// pitchedRefs are corpus clips with an unambiguous fundamental — sustained
// winds, bowed strings, and struck/plucked instruments whose pitch is the
// whole point of the recording.
var pitchedRefs = []string{
	"oboe-A4", "flute-A4", "clarinet-A4", "bassoon-A2", "sax-A4",
	"trumpet-A4", "trombone-A3", "french-horn-A3", "tuba-A2",
	"violin-A4", "viola-A4", "cello-A3", "double-bass-A2",
	"harp-vcsl-e5", "guitar-nylon-freesound", "piano-grand-vcsl-gs5",
	"marimba-vcsl", "vibraphone-vcsl", "xylophone-vcsl", "glockenspiel-vcsl",
	"kalimba-vcsl", "pan-flute-vcsl", "organ-church-vcsl", "harpsichord-vcsl",
}

// lowConfidencePitchedRefs are nominally pitched clips that score BELOW the
// threshold, recorded here deliberately rather than papered over.
//
// These are corpus-quality findings, not detector bugs. In each case YIN finds
// periodicity in only a small share of the clip's energy, which means the
// harmonic-series assumption really is shaky FOR THAT RECORDING — so reporting
// the harmonic descriptors as "not measurable" is the honest outcome, and the
// fix is a better reference clip, not a looser threshold. `refs doctor`
// surfaces the same numbers so they stay visible.
var lowConfidencePitchedRefs = map[string]string{
	"koto-vcsl":                  "fast pluck; detected 106.5 Hz may be an octave error (koto strings sit ~D3+)",
	"piano-felt-vcsl":            "felt-damped struck string; energy is dominated by an inharmonic attack",
	"sax-baritone-freesound-alt": "detected 344.5 Hz is plainly wrong for a baritone sax (~123 Hz) — clip needs replacing",
	"voice-ahh-freesound":        "detected 59.2 Hz is far below any sung pitch — clip needs replacing",
	"conga-freesound-alt":        "tuned drum, but this recording's pitch is not recoverable",
}

// refConfidence decodes a corpus reference and returns its F0 estimate.
// ok=false when the clip is not available on this machine.
func refConfidence(id string) (fingerprint.F0Estimate, bool, error) {
	p, ok := wavref.Resolve(id)
	if !ok {
		return fingerprint.F0Estimate{}, false, nil
	}
	s, sr, err := audio.DecodeWAVAtRate(p, wavref.Rate)
	if err != nil {
		return fingerprint.F0Estimate{}, false, fmt.Errorf("%s: decode %s: %w", id, p, err)
	}
	f := make([]float64, len(s))
	for i, v := range s {
		f[i] = float64(v)
	}
	return fingerprint.DetectF0Detailed(wave.Wave{Samples: f, SampleRate: sr}), true, nil
}

// checkF0ConfidenceSeparation: F0ReliableConfidence must sit strictly between
// the two populations, with real margin on both sides rather than threading a
// crowded boundary.
func checkF0ConfidenceSeparation(r *checkReport) error {
	type scored struct {
		id   string
		conf float64
	}
	var unpitched, pitched []scored

	for _, id := range unpitchedRefs {
		e, ok, err := refConfidence(id)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		unpitched = append(unpitched, scored{id, e.Confidence})
		if e.Reliable() {
			r.Errorf("%s is unpitched percussion but scored RELIABLE (confidence=%.3f, hz=%.2f) — "+
				"the harmonic descriptors would trust a fundamental that does not exist",
				id, e.Confidence, e.Hz)
		}
	}
	for _, id := range pitchedRefs {
		e, ok, err := refConfidence(id)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		pitched = append(pitched, scored{id, e.Confidence})
		if !e.Reliable() {
			r.Errorf("%s is unambiguously pitched but scored UNRELIABLE (confidence=%.3f, hz=%.2f) — "+
				"the harmonic descriptors would be suppressed on a real pitched instrument",
				id, e.Confidence, e.Hz)
		}
	}
	if len(unpitched) == 0 || len(pitched) == 0 {
		r.Skipf("needs both pitched and unpitched clips present (have %d pitched, %d unpitched)", len(pitched), len(unpitched))
		return nil
	}

	sort.Slice(unpitched, func(i, j int) bool { return unpitched[i].conf > unpitched[j].conf })
	sort.Slice(pitched, func(i, j int) bool { return pitched[i].conf < pitched[j].conf })
	hiUnpitched, loPitched := unpitched[0], pitched[0]

	r.Logf("threshold F0ReliableConfidence=%.2f", fingerprint.F0ReliableConfidence)
	r.Logf("  highest unpitched: %.3f (%s)", hiUnpitched.conf, hiUnpitched.id)
	r.Logf("  lowest pitched:    %.3f (%s)", loPitched.conf, loPitched.id)
	r.Logf("  empty margin:      %.3f", loPitched.conf-hiUnpitched.conf)

	if loPitched.conf <= hiUnpitched.conf {
		r.Errorf("the two populations OVERLAP: %s (unpitched) scores %.3f but %s (pitched) scores only %.3f — "+
			"no threshold can separate them",
			hiUnpitched.id, hiUnpitched.conf, loPitched.id, loPitched.conf)
		return nil
	}
	// Require the threshold to sit inside the gap with room on both sides, so
	// a small detector change cannot silently flip a clip across it.
	const minMargin = 0.10
	if fingerprint.F0ReliableConfidence-hiUnpitched.conf < minMargin {
		r.Errorf("threshold %.2f is only %.3f above the highest unpitched clip (%s) — want >= %.2f margin",
			fingerprint.F0ReliableConfidence, fingerprint.F0ReliableConfidence-hiUnpitched.conf, hiUnpitched.id, minMargin)
	}
	if loPitched.conf-fingerprint.F0ReliableConfidence < minMargin {
		r.Errorf("threshold %.2f is only %.3f below the lowest pitched clip (%s) — want >= %.2f margin",
			fingerprint.F0ReliableConfidence, loPitched.conf-fingerprint.F0ReliableConfidence, loPitched.id, minMargin)
	}
	return nil
}

// checkF0LowConfidenceOutliers keeps the documented corpus-quality outliers
// honest: if a clip is replaced and starts scoring reliably, this fails so the
// note in lowConfidencePitchedRefs gets deleted rather than rotting.
func checkF0LowConfidenceOutliers(r *checkReport) error {
	ids := make([]string, 0, len(lowConfidencePitchedRefs))
	for id := range lowConfidencePitchedRefs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		e, ok, err := refConfidence(id)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if e.Reliable() {
			r.Errorf("%s now scores RELIABLE (confidence=%.3f, hz=%.2f) — the recorded reason %q "+
				"no longer applies; remove it from lowConfidencePitchedRefs",
				id, e.Confidence, e.Hz, lowConfidencePitchedRefs[id])
			continue
		}
		r.Logf("%-28s confidence=%.3f hz=%8.2f  %s", id, e.Confidence, e.Hz, lowConfidencePitchedRefs[id])
	}
	return nil
}
