package fingerprint

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// TestCoreDescriptorsCoverTheNineLegacyAxes pins the Group/Name strings that
// cmd/refs/testdata/diagnose_baseline_2026-08-22.json and the mapping doc's
// verdict table key on. Renaming any of them breaks the acceptance gate.
func TestCoreDescriptorsCoverTheNineLegacyAxes(t *testing.T) {
	want := map[string]struct{ group, name string }{
		IDNoiseFraction:     {GroupNoise, "noise fraction"},
		IDNoiseFlatness:     {GroupNoise, "spectral flatness"},
		IDEnvAttack:         {GroupEnvelope, "attack"},
		IDEnvRingT60:        {GroupEnvelope, "ring T60"},
		IDEnvRelease:        {GroupEnvelope, "release"},
		IDInharmonicity:     {GroupPartials, "inharmonicity"},
		IDSeriesDeviation:   {GroupPartials, "partial series deviation"},
		IDCentroid:          {GroupBright, "centroid"},
		IDRolloff:           {GroupBright, "rolloff"},
		IDSustainModulation: {GroupMod, "sustain modulation"},
	}
	for id, w := range want {
		d, ok := DefaultRegistry().ByID(id)
		if !ok {
			t.Errorf("missing descriptor %q", id)
			continue
		}
		if d.Group != w.group || d.Name != w.name {
			t.Errorf("%s: got (%q,%q) want (%q,%q)", id, d.Group, d.Name, w.group, w.name)
		}
	}
}

// TestSilentWaveYieldsUnavailableNotZero is the honest-N/A contract: a silent
// clip must say it could not be measured, never report 0 as if measured.
func TestSilentWaveYieldsUnavailableNotZero(t *testing.T) {
	b := NewBundle(bundleSilence(44100, 1), "silent")
	for _, id := range []string{IDEnvAttack, IDEnvRingT60, IDEnvRelease, IDSustainModulation} {
		d, ok := DefaultRegistry().ByID(id)
		if !ok {
			t.Fatalf("missing descriptor %q", id)
		}
		m := d.Extract(b)
		if m.OK {
			t.Errorf("%s: a silent wave must yield an unavailable measurement", id)
		}
		if m.NA == "" {
			t.Errorf("%s: unavailable measurements must carry a reason", id)
		}
	}
}

// TestEnvelopeDescriptorsDoNotApplyToSilence checks the other half of the
// gate: silence should not even produce envelope rows.
func TestEnvelopeDescriptorsDoNotApplyToSilence(t *testing.T) {
	b := NewBundle(bundleSilence(44100, 1), "silent")
	for _, d := range DefaultRegistry().Applicable(b) {
		if d.Group == GroupEnvelope || d.ID == IDSustainModulation {
			t.Errorf("%s must not apply to silence", d.ID)
		}
	}
}

// coreGroups are the groups this smoke test covers: the whole-signal axes that
// are defined for ANY pitched tone.
//
// Deliberately not "every registered descriptor". A synthetic steady sine has
// no vibrato and no body resonance, so the vibrato and formant descriptors
// report an honest not-applicable on it -- which is the designed behaviour, not
// a failure. Asserting availability across the whole registry would force those
// descriptors to invent a zero for a signal that genuinely has neither.
var coreGroups = map[string]bool{
	GroupNoise: true, GroupEnvelope: true, GroupPartials: true,
	GroupBright: true, GroupMod: true,
}

// scaleDecayingToneWithNoiseOnset is a third probe, added alongside
// steady-tone and percussive-hit for noise.decay_vs_tone (task 6): a
// genuinely pitched signal (most of its length is a clean, slowly-decaying
// tone, so F0 detection stays confident) that ALSO carries a short,
// separately-decaying noise burst at the onset.
//
// Neither existing probe can exercise a TONE-VS-RESIDUAL DECAY RATIO
// descriptor: bundleTone's steady tone never decays at all (both its tonal
// AND its near-zero residual track are trivially "still above -20dB at clip
// end"), and scalePercussive mixes noise in PROPORTIONAL to the tone's own
// envelope throughout the clip, which is realistic for a percussive hit but
// depresses F0 confidence (0.25, below F0ReliableConfidence) enough that a
// descriptor correctly gated on RequiresPitched never even attempts it. This
// probe's noise decays independently and much faster than its tone, so both
// halves of a decay-ratio ARE observable before the clip ends, while the
// tone stays confidently pitched throughout.
func scaleDecayingToneWithNoiseOnset(sr int, f0, sec float64) wave.Wave {
	n := int(float64(sr) * sec)
	s := make([]float64, n)
	rng := rand.New(rand.NewSource(13))
	for i := range s {
		t := float64(i) / float64(sr)
		tone := math.Exp(-t/0.5) * 0.5 * math.Sin(2*math.Pi*f0*t)
		noise := math.Exp(-t/0.02) * 0.4 * (rng.Float64()*2 - 1)
		s[i] = tone + noise
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// TestCoreDescriptorsExtractOnARealSignal is a smoke test that every core
// descriptor produces an available measurement on at least one ordinary signal.
//
// "At least one" rather than "on a steady tone", because the probes cover
// genuinely different definitions. A dead-steady tone never decays, so the
// decay-threshold descriptors correctly report a lower bound instead of a
// measurement on it; a percussive hit has no sustain plateau, so the sustain
// descriptors correctly decline there; neither probe gives a tone-vs-residual
// decay-RATIO descriptor anything to divide (see scaleDecayingToneWithNoiseOnset's
// own doc). Requiring availability on ALL would force one of those honest
// not-applicables to become a fabricated number, which is exactly the
// discipline Measurement.OK exists to prevent. Requiring it on NONE would let
// a descriptor that never measures anything slip in.
func TestCoreDescriptorsExtractOnARealSignal(t *testing.T) {
	probes := map[string]*Bundle{
		"steady-tone":              NewBundle(bundleTone(44100, 220, 2), "tone"),
		"percussive-hit":           NewBundle(scalePercussive(44100, 220, 2), "hit"),
		"decaying-tone-with-onset": NewBundle(scaleDecayingToneWithNoiseOnset(44100, 220, 2), "decay-onset"),
	}

	availableSomewhere := map[string]bool{}
	seen := map[string]bool{}
	for name, b := range probes {
		for _, d := range DefaultRegistry().Applicable(b) {
			if !coreGroups[d.Group] {
				continue
			}
			seen[d.ID] = true
			m := d.Extract(b)
			if !m.OK {
				continue
			}
			availableSomewhere[d.ID] = true
			if math.IsNaN(m.Primary()) || math.IsInf(m.Primary(), 0) {
				t.Errorf("%s on %s: produced a non-finite value %v", d.ID, name, m.Primary())
			}
		}
	}
	for id := range seen {
		if !availableSomewhere[id] {
			t.Errorf("%s: unavailable on BOTH a steady tone and a percussive hit -- "+
				"a core descriptor that never measures anything is not earning its row", id)
		}
	}
}

// TestNoDescriptorProducesNonFiniteValues applies to the WHOLE registry, on
// every probe signal. A descriptor may legitimately be unavailable; it may
// never report NaN or +/-Inf, which would poison a comparison silently rather
// than declaring itself unmeasurable.
func TestNoDescriptorProducesNonFiniteValues(t *testing.T) {
	for _, probe := range scaleProbes() {
		b := NewBundle(probe.w, probe.name)
		for _, d := range DefaultRegistry().Applicable(b) {
			m := d.Extract(b)
			if !m.OK {
				continue
			}
			for i, v := range append([]float64{m.Primary()}, m.Values...) {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Errorf("%s on %s: component %d is non-finite (%v)", d.ID, probe.name, i, v)
				}
			}
		}
	}
}

// TestIdenticalBundlesProduceNoMismatch is the identity invariant: comparing a
// signal against itself must yield "ok" on every axis. If any descriptor
// fails this, its comparator is asymmetric or non-deterministic.
func TestIdenticalBundlesProduceNoMismatch(t *testing.T) {
	w := bundleTone(44100, 220, 2)
	a := NewBundle(w, "a")
	b := NewBundle(w, "b")
	for _, d := range DefaultRegistry().Applicable(a) {
		delta := d.CompareWith(d.Extract(a), d.Extract(b))
		if !delta.OK {
			continue // legitimately unavailable
		}
		if delta.Verdict != VerdictOK {
			t.Errorf("%s: self-comparison gave %v (%s), want ok", d.ID, delta.Verdict, delta.Display)
		}
	}
}

// TestRingT60EscalatesWhenBothSidesExtrapolated pins the compound-measurement
// path: two clips that both still ring at clip end must be compared by decay
// RATE, not by their equally-clamped T60 lower bounds.
func TestRingT60EscalatesWhenBothSidesExtrapolated(t *testing.T) {
	// Both clamped to the same T60 bound, but decay rates differ 4x — a raw
	// ratio would report a vacuous "ok".
	ref := Vector([]float64{1.95, 1, -0.7}, nil)
	synth := Vector([]float64{1.95, 1, -3.1}, nil)
	rtDrift, rtMismatch := ringT60Thresholds()
	d := compareRingT60(ref, synth, rtDrift, rtMismatch)
	if d.Verdict == VerdictOK {
		t.Fatal("both-extrapolated with 4x differing decay rates must not read ok")
	}
	if d.Direction != "low" {
		t.Errorf("direction: got %q want low (synth decays faster => shorter ring)", d.Direction)
	}
}

func TestRingT60UsesPlainRatioWhenObservable(t *testing.T) {
	ref := Vector([]float64{1.130, 0, -50}, nil)
	synth := Vector([]float64{0.160, 0, -400}, nil)
	rtDrift, rtMismatch := ringT60Thresholds()
	d := compareRingT60(ref, synth, rtDrift, rtMismatch)
	if d.Verdict != VerdictMismatch || d.Direction != "low" {
		t.Fatalf("got %v/%q want MISMATCH/low", d.Verdict, d.Direction)
	}
	if d.Display != "7.1x short" {
		t.Errorf("display: got %q want %q", d.Display, "7.1x short")
	}
}

// ringT60Thresholds returns the pair CompareWith hands compareRingT60, so
// these unit tests exercise the comparator with exactly the numbers
// production uses. env.ring_t60 is Demoted and therefore excluded from
// threshold calibration, so this is stable at the KindSeconds defaults.
func ringT60Thresholds() (float64, float64) {
	d, ok := DefaultRegistry().ByID(IDEnvRingT60)
	if !ok {
		return KindSeconds.Thresholds()
	}
	return d.Thresholds()
}
