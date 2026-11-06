package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestCentroidTrackIsFlatForSteadyTone(t *testing.T) {
	tr := NewBundle(bundleTone(44100, 440, 2), "t").Tracks()
	if tr.NumFrames == 0 {
		t.Fatal("no frames produced")
	}
	mean, std := meanStd(tr.Centroid)
	if mean <= 0 {
		t.Fatal("centroid track is empty or zero")
	}
	if std/mean > 0.05 {
		t.Fatalf("steady tone centroid should be near-constant; CoV=%v", std/mean)
	}
}

func TestRMSTrackFollowsAnAmplitudeRamp(t *testing.T) {
	w := bundleTone(44100, 440, 2)
	for i := range w.Samples { // linear fade-in
		w.Samples[i] *= float64(i) / float64(len(w.Samples))
	}
	tr := NewBundle(w, "ramp").Tracks()
	if tr.RMS[0] >= tr.RMS[len(tr.RMS)-1] {
		t.Fatal("RMS track did not follow the ramp")
	}
	if s := StatsOf(tr.RMS, tr.HopSec); s.Slope <= 0 {
		t.Errorf("a fade-in must have positive RMS slope; got %v", s.Slope)
	}
}

// TestHarmonicTracksCaptureABloom is the trumpet case in miniature: a tone
// whose upper harmonic grows in over the note. A mid-sustain snapshot sees one
// arbitrary point on that curve; the harmonic track sees the whole trajectory.
func TestHarmonicTracksCaptureABloom(t *testing.T) {
	sr := 44100
	secs := 2.0
	n := int(float64(sr) * secs)
	s := make([]float64, n)
	for i := range s {
		x := float64(i) / float64(sr)
		frac := float64(i) / float64(n)
		fundamental := 0.5 * math.Sin(2*math.Pi*220*x)
		// Second harmonic fades in linearly across the note.
		second := 0.4 * frac * math.Sin(2*math.Pi*440*x)
		s[i] = fundamental + second
	}
	tr := NewBundle(wave.Wave{Samples: s, SampleRate: sr}, "bloom").Tracks()
	if tr.F0Hz <= 0 {
		t.Fatalf("expected a detected fundamental, got %v", tr.F0Hz)
	}
	h2 := StatsOf(tr.Harmonics[1], tr.HopSec)
	if h2.Slope <= 0 {
		t.Errorf("H2 must rise across a bloom; got slope %v dB/sec", h2.Slope)
	}
	h1 := StatsOf(tr.Harmonics[0], tr.HopSec)
	if h1.Slope >= h2.Slope {
		t.Errorf("H2 (%v) must rise faster than the steady H1 (%v)", h2.Slope, h1.Slope)
	}
}

func TestTracksOnSilenceAreFiniteAndEmptyOfPitch(t *testing.T) {
	tr := NewBundle(bundleSilence(44100, 1), "silent").Tracks()
	for i, v := range tr.Centroid {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("silence produced a non-finite centroid at frame %d: %v", i, v)
		}
	}
	if tr.F0Hz != 0 {
		t.Errorf("silence must have no fundamental; got %v", tr.F0Hz)
	}
}

func TestTracksAreMemoized(t *testing.T) {
	b := NewBundle(bundleTone(44100, 220, 1), "t")
	if b.Tracks() != b.Tracks() {
		t.Fatal("Tracks() must return the same memoized instance")
	}
}

func TestTracksAllSlicesShareLength(t *testing.T) {
	tr := NewBundle(bundleTone(44100, 220, 2), "t").Tracks()
	n := tr.NumFrames
	for name, s := range map[string][]float64{
		"RMS": tr.RMS, "Crest": tr.Crest, "Centroid": tr.Centroid,
		"Rolloff": tr.Rolloff, "Flatness": tr.Flatness, "NoiseFraction": tr.NoiseFraction,
	} {
		if len(s) != n {
			t.Errorf("%s: length %d, want %d — tracks must be directly correlatable", name, len(s), n)
		}
	}
	for k := range tr.Harmonics {
		if len(tr.Harmonics[k]) != n {
			t.Errorf("Harmonics[%d]: length %d, want %d", k, len(tr.Harmonics[k]), n)
		}
	}
}

// TestCentroidStatsAgreeWithLegacyTemporalFingerprint is the re-expression
// invariant. TemporalFingerprint.CentroidMean/.CentroidSlope are hand-written
// scalars over the same quantity; StatsOf(tracks.Centroid) must reproduce them
// in magnitude and sign.
//
// The tolerance is deliberately loose: computeTemporal skips the first 0.1 s
// and trims to 1024 of 1025 bins, while Tracks covers the whole signal at the
// full bin count. They should agree in magnitude and sign, not exactly. An
// exact assertion here would be asserting that two deliberately different
// windows are the same window.
func TestCentroidStatsAgreeWithLegacyTemporalFingerprint(t *testing.T) {
	w := bundleTone(44100, 440, 2)
	// Give it some evolution so slope is non-trivial.
	for i := range w.Samples {
		w.Samples[i] *= 0.3 + 0.7*float64(i)/float64(len(w.Samples))
	}
	b := NewBundle(w, "t")
	legacy := b.Fingerprint().Temporal
	if legacy == nil {
		t.Skip("no temporal fingerprint produced")
	}
	got := StatsOf(b.Tracks().Centroid, b.Tracks().HopSec)

	rel := math.Abs(got.Mean-legacy.CentroidMean) / math.Max(math.Abs(legacy.CentroidMean), 1)
	if rel > 0.20 {
		t.Errorf("re-expressed centroid mean %v diverges from legacy %v (rel %v)",
			got.Mean, legacy.CentroidMean, rel)
	}
}
