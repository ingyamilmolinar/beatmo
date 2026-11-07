package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Every reference is truncated (2–3 s), so a ringing note ends long before -60 dB and both sides report a clamped
// T60 that reads "fine" on instruments the listener rejected; ring T60 was anti-correlated with the ear verdicts.
// Shallower thresholds are reached inside the clip, so they measure what a listener hears.

// Known T-values, so the assertions are arithmetic, not snapshots.
func expDecay(sr int, f0, sec, dbPerSec float64) wave.Wave {
	n := int(float64(sr) * sec)
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		amp := math.Pow(10, -dbPerSec*t/20)
		s[i] = 0.9 * amp * math.Sin(2*math.Pi*f0*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestDecayProfile_MatchesAnalyticDecayRate(t *testing.T) {
	// 20 dB/sec: T10 at 0.5 s, T20 at 1.0 s, T30 at 1.5 s.
	seg := AnalyzeEnvelopeSegments(expDecay(44100, 220, 3.0, 20))
	for _, tc := range []struct {
		name string
		got  float64
		want float64
	}{
		{"T10", seg.DecayT10Sec, 0.5},
		{"T20", seg.DecayT20Sec, 1.0},
		{"T30", seg.DecayT30Sec, 1.5},
	} {
		if math.Abs(tc.got-tc.want) > 0.08 {
			t.Errorf("%s = %.3fs, want ~%.2fs for a 20 dB/s decay", tc.name, tc.got, tc.want)
		}
	}
}

// A clip ending before -60 dB still yields MEASURED shallow thresholds.
func TestDecayProfile_ShallowThresholdsSurviveClipTruncation(t *testing.T) {
	// 8 dB/sec over a 2 s clip reaches only -16 dB: T10 measurable, T20 not.
	seg := AnalyzeEnvelopeSegments(expDecay(44100, 220, 2.0, 8))
	if seg.DecayT10Extrapolated {
		t.Errorf("T10 must be MEASURED within a 2s clip decaying at 8 dB/s, got %.3fs (extrapolated)", seg.DecayT10Sec)
	}
	if math.Abs(seg.DecayT10Sec-1.25) > 0.12 {
		t.Errorf("T10 = %.3fs, want ~1.25s", seg.DecayT10Sec)
	}
	if !seg.DecayT20Extrapolated {
		t.Error("T20 cannot be reached in this clip and must be flagged extrapolated, not reported as measured")
	}
	if !seg.RingT60Extrapolated {
		t.Error("T60 must still be flagged extrapolated (the pre-existing behaviour)")
	}
}

// Each equal slice of the audible span's share of the energy.
func TestEnergySections_SumToOneAndTrackShape(t *testing.T) {
	seg := AnalyzeEnvelopeSegments(expDecay(44100, 220, 2.0, 30))
	var sum float64
	for _, v := range seg.EnergySections {
		sum += v
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Errorf("energy sections sum to %.6f, want 1", sum)
	}
	if len(seg.EnergySections) != energySectionCount {
		t.Fatalf("got %d sections, want %d", len(seg.EnergySections), energySectionCount)
	}
	// A decaying hit is front-loaded: strictly descending shares.
	for i := 1; i < len(seg.EnergySections); i++ {
		if seg.EnergySections[i] > seg.EnergySections[i-1] {
			t.Errorf("section %d (%.4f) exceeds section %d (%.4f) on a monotonically decaying hit",
				i, seg.EnergySections[i], i-1, seg.EnergySections[i-1])
		}
	}
	if seg.EnergySections[0] < 0.5 {
		t.Errorf("a 30 dB/s decay must put most energy in the first section, got %.3f", seg.EnergySections[0])
	}
}

func TestEnergySections_FlatOnASustainedTone(t *testing.T) {
	n := 44100 * 2
	s := make([]float64, n)
	for i := range s {
		s[i] = 0.5 * math.Sin(2*math.Pi*220*float64(i)/44100)
	}
	seg := AnalyzeEnvelopeSegments(wave.Wave{Samples: s, SampleRate: 44100})
	for i, v := range seg.EnergySections {
		if math.Abs(v-1.0/float64(energySectionCount)) > 0.05 {
			t.Errorf("section %d = %.3f on a steady tone; every section should hold ~%.3f",
				i, v, 1.0/float64(energySectionCount))
		}
	}
}

func TestDecayProfile_SilentClipReportsNothing(t *testing.T) {
	seg := AnalyzeEnvelopeSegments(wave.Wave{Samples: make([]float64, 4410), SampleRate: 44100})
	if !seg.Silent {
		t.Fatal("an all-zero clip must be Silent")
	}
	if seg.DecayT10Sec != 0 || len(seg.EnergySections) != 0 {
		t.Errorf("a silent clip must report no decay profile, got T10=%v sections=%v",
			seg.DecayT10Sec, seg.EnergySections)
	}
}
