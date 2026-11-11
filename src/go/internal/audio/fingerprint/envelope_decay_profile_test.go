package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// envelope_decay_profile_test.go — decay measured in SECTIONS, not as one T60.
//
// WHY T60 WAS NOT ENOUGH
// ----------------------
// RingT60Sec is the time to fall 60 dB below the peak. Every instrument
// reference in the corpus is truncated by its own fetch config (convert.max_sec,
// typically 2-3 s), so for anything that rings the clip ends long before -60 dB
// and BOTH sides report a clamped lower bound pinned to the clip length. The
// comparison then reads "1.1x short -- fine" on instruments the listener
// rejected outright:
//
//	harp           ref T60 1.95  synth T60 1.76   "1.1x short"   listener: "n"
//	tubular-bells  rate -4.3 vs -5.0 dB/s          "fine"         listener: "n"
//
// Validated against the ear verdicts, `ring T60` scored AUC 0.21 (95% CI
// 0.00-0.47) -- confidently ANTI-correlated with perceived quality.
//
// The shallower thresholds are reached WELL INSIDE the clip, so they measure
// rather than clamp, and they describe the part of the decay a listener
// actually hears:
//
//	harp           T20 ref 0.48  synth 1.56   3.3x too long
//	tubular-bells  T10 ref 1.28  synth 0.35   3.7x too fast
//
// Both are exactly the complaints the listener made and T60 could not see.

// expDecay builds a hit decaying at a known dB/sec, so the T-values are known
// analytically and the test asserts against arithmetic rather than a snapshot.
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

// The whole point: a clip that ends before -60 dB must still yield MEASURED
// shallow thresholds rather than clamped ones.
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

// EnergySections is the "split it up into multiple sections" measurement: what
// share of the note's energy lands in each equal slice of its audible span.
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
