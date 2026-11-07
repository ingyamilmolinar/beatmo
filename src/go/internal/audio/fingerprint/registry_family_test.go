package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Peaks at once and never plateaus.
func percussiveHit(sr int, secs float64) wave.Wave {
	n := int(float64(sr) * secs)
	s := make([]float64, n)
	// A fixed LCG, so the fixture never varies and threshold tests stay stable.
	state := uint32(12345)
	for i := range s {
		state = state*1664525 + 1013904223
		noise := float64(int32(state))/float64(math.MaxInt32) - 0.0
		decay := math.Exp(-12 * float64(i) / float64(sr))
		s[i] = 0.8 * noise * decay
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// They fire on the character they describe and stay silent elsewhere.
func TestFamilyDescriptorsAreGated(t *testing.T) {
	tone := NewBundle(bundleTone(44100, 220, 2), "tone")
	for _, d := range DefaultRegistry().All() {
		if d.Group != GroupSnare {
			continue
		}
		if d.Applies(tone) {
			t.Errorf("%s must not apply to a sustained tone", d.ID)
		}
	}

	hit := NewBundle(percussiveHit(44100, 0.5), "hit")
	var snareRows int
	for _, d := range DefaultRegistry().Applicable(hit) {
		if d.Group == GroupSnare {
			snareRows++
		}
	}
	if snareRows == 0 {
		t.Error("no percussive-body descriptors applied to a decaying noise burst")
	}
}

func TestPianoDescriptorsRequirePitch(t *testing.T) {
	silent := NewBundle(bundleSilence(44100, 1), "silent")
	for _, d := range DefaultRegistry().Applicable(silent) {
		if d.Group == GroupPiano {
			t.Errorf("%s must not apply to silence", d.ID)
		}
	}
}

// Reachable through the registry, not only their own tests.
func TestFamilyDescriptorsRegistered(t *testing.T) {
	for _, id := range []string{
		IDSnareBodyFund, IDSnareToneNoise, IDSnareLowBody, IDSnareHighBuzz,
		IDSnareBodyDecay, IDSnareBuzzDecay,
		IDPianoInharmonicity, IDPianoTwoStage, IDPianoDecayFast, IDPianoDecaySlow, IDPianoStretch,
		IDWindAttackTime, IDWindCentroidSlope, IDWindCentroidTraj, IDWindTemporalCent,
		IDSpectralIrregular,
	} {
		if _, ok := DefaultRegistry().ByID(id); !ok {
			t.Errorf("descriptor %q is not registered", id)
		}
	}
}

// On signals they were never exercised against.
func TestFamilyExtractionIsFinite(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    *Bundle
	}{
		{"tone", NewBundle(bundleTone(44100, 220, 2), "tone")},
		{"hit", NewBundle(percussiveHit(44100, 0.5), "hit")},
	} {
		for _, d := range DefaultRegistry().Applicable(tc.b) {
			m := d.Extract(tc.b)
			if !m.OK {
				continue
			}
			vals := m.Values
			if m.Form == FormScalar {
				vals = []float64{m.Scalar}
			}
			for i, v := range vals {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Errorf("%s/%s: component %d is non-finite (%v)", tc.name, d.ID, i, v)
				}
			}
		}
	}
}

// Otherwise a report would re-run every family analyser per descriptor.
func TestBundleMemoizesFamilyAnalyzers(t *testing.T) {
	b := NewBundle(percussiveHit(44100, 0.5), "hit")
	a1, a2 := b.Snare(), b.Snare()
	if a1 != a2 {
		t.Error("Snare() is not stable across calls")
	}
	w1, w2 := b.WindAttack(), b.WindAttack()
	if w1 != w2 {
		t.Error("WindAttack() is not stable across calls")
	}
}
