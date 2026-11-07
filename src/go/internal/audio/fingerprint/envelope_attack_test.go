package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// The attack is the ONSET RISE, not the time to the loudest sample: a tone with tremolo, a bow swell or beating
// peaks mid-note, and onset → global maximum then measures the swell (synth strings read ~1.6 s against 0.1–0.2 s).

// A fast onset to a local peak, a dip well below it, then a slow swell to a LATER, HIGHER maximum, modelled on the
// measured synth cello: a probe that merely rises to a late peak has no dip and never exercises the rule.
func attackSwellProbe(sr int, f0, sec float64) wave.Wave {
	n := int(float64(sr) * sec)
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		var amp float64
		switch {
		case t < 0.05: // 50 ms onset rise to the local peak
			amp = t / 0.05
		case t < 0.6: // decay into the trough
			amp = 1.0 - 0.75*(t-0.05)/0.55
		default: // slow swell past the original peak
			amp = 0.25 + 1.05*(t-0.6)/math.Max(sec-0.6, 1e-9)
		}
		s[i] = 0.4 * amp * math.Sin(2*math.Pi*f0*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// The case the global-peak rule always got right, and must keep right.
func attackHitProbe(sr int, f0, sec float64) wave.Wave {
	n := int(float64(sr) * sec)
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		var rise float64
		if t < 0.005 {
			rise = t / 0.005
		} else {
			rise = math.Exp(-(t - 0.005) * 10)
		}
		s[i] = 0.8 * rise * math.Sin(2*math.Pi*f0*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// A 5 ms rise to a plateau rippling ±0.4 dB whose loudest crest is near the end: which crest wins depended on the
// sample rate.
func attackPlateauProbe(sr int, f0, sec float64) wave.Wave {
	n := int(float64(sr) * sec)
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		amp := 1 + 0.05*math.Sin(2*math.Pi*5*(t-0.005)) + 0.03*t
		if t < 0.005 {
			amp = t / 0.005
		}
		s[i] = 0.4 * amp * math.Sin(2*math.Pi*f0*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// No dip reaches 12 %, so the rise ends once it is within that tolerance of the top and stops rising for
// attackRiseMinSec, not at whichever crest is loudest.
func TestAttackSec_EndsWhereARipplingPlateauBegins(t *testing.T) {
	for _, sr := range []int{44100, 48000} {
		seg := AnalyzeEnvelopeSegments(attackPlateauProbe(sr, 110, 2.0))
		if seg.AttackSec > 0.12 {
			t.Errorf("%d Hz: AttackSec = %.3fs for a 5 ms rise onto a rippling plateau (loudest crest at %.3fs) — "+
				"the attack is being measured to a ripple crest", sr, seg.AttackSec, seg.PeakSec)
		}
		if seg.PeakSec < 1.0 {
			t.Errorf("%d Hz: PeakSec = %.3fs; the loudest crest is late and must still be reported there", sr, seg.PeakSec)
		}
	}
}

func TestAttackSec_IgnoresALaterLouderSwell(t *testing.T) {
	seg := AnalyzeEnvelopeSegments(attackSwellProbe(44100, 220, 2.0))
	if seg.Silent {
		t.Fatal("probe must not be silent")
	}
	// Generous slack for block-RMS resolution, nothing near the global-peak reading.
	if seg.AttackSec > 0.30 {
		t.Errorf("AttackSec = %.3fs for a 50 ms onset followed by a slow swell -- "+
			"the attack is still being measured to the global peak (PeakSec=%.3fs)",
			seg.AttackSec, seg.PeakSec)
	}
	// The swell is still real and must remain observable, just not as "attack".
	if seg.PeakSec < 0.5 {
		t.Errorf("PeakSec = %.3fs; the global peak genuinely is late and must still be reported there", seg.PeakSec)
	}
}

func TestAttackSec_UnchangedForAPercussiveHit(t *testing.T) {
	seg := AnalyzeEnvelopeSegments(attackHitProbe(44100, 220, 1.5))
	if seg.Silent {
		t.Fatal("probe must not be silent")
	}
	if seg.AttackSec > 0.08 {
		t.Errorf("AttackSec = %.3fs for a 5 ms percussive onset -- the fix must not "+
			"lengthen attacks that were already correct", seg.AttackSec)
	}
}

// Distinct from PeakSec exactly when a later swell exists, and that gap is worth reporting.
func TestAttackPeakSec_SeparatesOnsetPeakFromGlobalPeak(t *testing.T) {
	mod := AnalyzeEnvelopeSegments(attackSwellProbe(44100, 220, 2.0))
	if mod.AttackPeakSec >= mod.PeakSec {
		t.Errorf("AttackPeakSec=%.3f must precede PeakSec=%.3f on a swelling signal",
			mod.AttackPeakSec, mod.PeakSec)
	}
	hit := AnalyzeEnvelopeSegments(attackHitProbe(44100, 220, 1.5))
	if math.Abs(hit.AttackPeakSec-hit.PeakSec) > 0.05 {
		t.Errorf("on a percussive hit the onset peak IS the global peak, got %.3f vs %.3f",
			hit.AttackPeakSec, hit.PeakSec)
	}
}

// The real instruments that exposed the bug.
func TestAttackSec_ShippedCelloAndViolinNoLongerReportASecondLongAttack(t *testing.T) {
	for _, tc := range []struct {
		name string
		w    wave.Wave
	}{
		{"swelling-cello-like", attackSwellProbe(44100, 110, 2.0)},
		{"swelling-violin-like", attackSwellProbe(44100, 220, 2.0)},
	} {
		seg := AnalyzeEnvelopeSegments(tc.w)
		if seg.AttackSec > 0.5 {
			t.Errorf("%s: AttackSec=%.3fs -- a bowed onset is tenths of a second, not seconds",
				tc.name, seg.AttackSec)
		}
	}
}
