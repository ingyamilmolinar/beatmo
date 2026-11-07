package fingerprint

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// The baseline and the verdict table key on these strings.
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

// A silent clip says it could not be measured, never 0.
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

func TestEnvelopeDescriptorsDoNotApplyToSilence(t *testing.T) {
	b := NewBundle(bundleSilence(44100, 1), "silent")
	for _, d := range DefaultRegistry().Applicable(b) {
		if d.Group == GroupEnvelope || d.ID == IDSustainModulation {
			t.Errorf("%s must not apply to silence", d.ID)
		}
	}
}

// Not every descriptor: a steady sine has no vibrato or body resonance, so those report an honest N/A by design.
var coreGroups = map[string]bool{
	GroupNoise: true, GroupEnvelope: true, GroupPartials: true,
	GroupBright: true, GroupMod: true,
}

// A clean, slowly decaying, confidently pitched tone with an independently and faster decaying noise burst at the
// onset: the other probes never let a tone-vs-residual decay ratio observe both halves.
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

// On at least one probe: requiring all would force an honest N/A into a fabricated number, requiring none would admit
// a descriptor that never measures.
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

// Unavailable is allowed; NaN or ±Inf would poison a comparison silently.
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

// A failure means an asymmetric or non-deterministic comparator.
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

// Both clips still ring at the clip end, so the comparison is by decay rate.
func TestRingT60EscalatesWhenBothSidesExtrapolated(t *testing.T) {
	// Equally clamped bounds, rates 4× apart: a raw ratio would say "ok".
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

// The pair production passes; env.ring_t60 is Demoted, so calibration leaves it at the defaults.
func ringT60Thresholds() (float64, float64) {
	d, ok := DefaultRegistry().ByID(IDEnvRingT60)
	if !ok {
		return KindSeconds.Thresholds()
	}
	return d.Thresholds()
}
