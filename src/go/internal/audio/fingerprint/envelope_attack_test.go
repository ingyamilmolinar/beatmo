package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// envelope_attack_test.go — the attack must be the ONSET RISE, not the time to
// whichever sample happens to be loudest.
//
// THE BUG THIS PINS
// -----------------
// AttackSec was onset -> argmax(env), the GLOBAL envelope maximum. That is
// correct only for a signal whose loudest moment IS its initial peak. Any
// sustained tone carrying amplitude modulation -- tremolo, a bow swell, two
// partials beating -- has its global maximum somewhere in the middle, and the
// "attack" then measures the swell.
//
// Measured on the shipped instruments: the synth cello reported a 1.62 s attack
// and the synth violin 1.70 s, against references at 0.09-0.18 s, producing a
// headline "18.6x too long" that was pure artifact. Both signals actually rise
// within ~200 ms and then dip and swell back to a later, higher peak. The
// diagnostic's single most-fired axis (37 of 57 instruments) was partly
// measuring this.

// attackSwellProbe reproduces the shape measured on the shipped cello: a fast
// onset to a local peak, a dip to well below it, then a slow swell to a LATER,
// HIGHER global maximum.
//
// The per-100ms mean-square of the real synth cello was
//
//	0.409 0.519 0.460 0.300 0.263 0.163 0.107 0.106 0.130 0.138 0.232 ... 0.636
//
// i.e. it starts strong, troughs at ~600 ms, and crests at ~1.6 s. A probe that
// merely rises monotonically to a late peak does NOT exercise the bug -- there
// is no dip to find -- so the envelope here is modelled on the measurement.
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

// attackHitProbe peaks immediately and decays -- the case where attack-to-global
// -peak was always right, and must stay right.
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

// attackPlateauProbe rises in 5 ms to a plateau that ripples ±0.4 dB (a bow,
// vibrato) and drifts up a little, so its LOUDEST crest is near the end. The
// shape is the shipped double bass at Fade-in 0: within 1.3 dB of full level
// from the first 10 ms, crests 0.1 dB apart — at 48 kHz the 180 ms crest won
// by 0.1 dB and the attack read 0.18 s, at 44.1 kHz the 80 ms one did.
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

// TestAttackSec_EndsWhereARipplingPlateauBegins: no dip ever reaches 12 %, so
// the rule used to fall back to the GLOBAL peak — whichever ripple crest
// happens to be loudest, anywhere in the note. Once the rise is within that
// tolerance of the top and stops rising for attackRiseMinSec, it has ended.
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
	// The onset rise is 50 ms. Allow generous slack for block-RMS resolution,
	// but nothing close to the ~1.6 s the global-peak rule produced.
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

// AttackPeakSec is the local peak the attack rise terminates at. It is distinct
// from PeakSec (the global maximum) precisely when a later swell exists, and
// that gap is itself worth reporting.
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
