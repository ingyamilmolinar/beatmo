package fingerprint

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Each descriptor's thresholds must be in the units it measures: a vector of HERTZ classified against KindFraction's
// defaults scored a 45 Hz match as MISMATCH. With no reference audio, a perturbation well INSIDE the mismatch
// threshold must read ok and one well PAST it must not, in the descriptor's own units.

// Together every applicability predicate fires.
func scaleProbes() []struct {
	name string
	w    wave.Wave
} {
	return []struct {
		name string
		w    wave.Wave
	}{
		{"sustained-tone", scaleTone(44100, 220, 2.0)},
		{"percussive-hit", scalePercussive(44100, 220, 1.5)},
		{"noisy-tone", scaleNoisyTone(44100, 440, 1.5)},
	}
}

func scaleTone(sr int, f0, sec float64) wave.Wave {
	n := int(float64(sr) * sec)
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = 0.5*math.Sin(2*math.Pi*f0*t) + 0.25*math.Sin(4*math.Pi*f0*t) + 0.12*math.Sin(6*math.Pi*f0*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func scalePercussive(sr int, f0, sec float64) wave.Wave {
	n := int(float64(sr) * sec)
	s := make([]float64, n)
	rng := rand.New(rand.NewSource(7))
	for i := range s {
		t := float64(i) / float64(sr)
		env := math.Exp(-t * 12)
		s[i] = env * (0.6*math.Sin(2*math.Pi*f0*t) + 0.4*(rng.Float64()*2-1))
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func scaleNoisyTone(sr int, f0, sec float64) wave.Wave {
	n := int(float64(sr) * sec)
	s := make([]float64, n)
	rng := rand.New(rand.NewSource(11))
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = 0.5*math.Sin(2*math.Pi*f0*t) + 0.15*(rng.Float64()*2-1)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// Ratio kinds compare larger/smaller, so their perturbation is multiplicative; every other kind is an absolute delta.
func perturb(d Descriptor, m Measurement, frac float64) Measurement {
	_, mismatch := d.Thresholds()
	out := m
	switch d.Kind {
	case KindSeconds, KindRatio:
		// frac=0 leaves it alone, frac=1 lands exactly on the mismatch ratio.
		scale := 1 + frac*(mismatch-1)
		out.Scalar = m.Scalar * scale
		if len(m.Values) > 0 {
			out.Values = append([]float64(nil), m.Values...)
			if d.Scope == ScopeBand {
				// A band vector's comparator reduces by mean over peer bands, so moving index 0 alone is diluted away; every band moves
				// by the same ratio.
				for i := range out.Values {
					out.Values[i] *= scale
				}
			} else {
				out.Values[0] = m.Values[0] * scale
			}
		}
	case KindHz:
		// KindHz scalars compare as a PERCENTAGE of the reference.
		if m.Form == FormScalar {
			out.Scalar = m.Scalar * (1 + frac*mismatch/100)
			break
		}
		out.Values = shiftAll(m.Values, frac*mismatch, d.Compound)
	default:
		if m.Form == FormScalar {
			out.Scalar = m.Scalar + frac*mismatch
			break
		}
		out.Values = shiftAll(m.Values, frac*mismatch, d.Compound)
	}
	return out
}

// A COMPOUND measurement's later components are flags and fallbacks, so only its headline moves; "has a custom
// comparator" was the wrong proxy.
func shiftAll(vs []float64, by float64, headlineOnly bool) []float64 {
	out := append([]float64(nil), vs...)
	if len(out) == 0 {
		return out
	}
	if headlineOnly {
		out[0] += by
		return out
	}
	for i := range out {
		out[i] += by
	}
	return out
}

// Two legitimate no-ops: a probe measuring exactly 0 cannot move multiplicatively, and a compound comparator that
// escalated to an untouchable component ignores the headline. minCheckedDescriptors stops the skips from swallowing
// the battery.
func reaches(d Descriptor, m, perturbed Measurement) bool {
	base := d.CompareWith(m, m)
	moved := d.CompareWith(m, perturbed)
	return base.Display != moved.Display || base.Norm != moved.Norm || base.Verdict != moved.Verdict
}

// Otherwise a change making every perturbation a no-op would pass while checking nothing.
const minCheckedDescriptors = 40

// TestDescriptorThresholdsAreOnTheRightScale is the gate.
func TestDescriptorThresholdsAreOnTheRightScale(t *testing.T) {
	// The gap leaves room for a comparator that folds or clamps near the boundary.
	const insideFrac = 0.1
	const pastFrac = 3.0
	checked := 0

	for _, probe := range scaleProbes() {
		probe := probe
		t.Run(probe.name, func(t *testing.T) {
			b := NewBundle(probe.w, probe.name)
			for _, d := range DefaultRegistry().Applicable(b) {
				m := d.Extract(b)
				if !m.OK {
					continue // honest not-applicable on this probe
				}

				if got := d.CompareWith(m, m); got.OK && got.Verdict != VerdictOK {
					t.Errorf("%s [%s]: identical measurements compared as %q (norm=%.3f) -- "+
						"a descriptor must agree with itself",
						d.ID, d.Kind, got.Verdict, got.Norm)
				}

				drift, mismatch := d.Thresholds()
				if !reaches(d, m, perturb(d, m, pastFrac)) {
					t.Logf("%s [%s]: perturbation does not reach the comparator on this probe "+
						"(measured %.4g) -- skipped", d.ID, d.Kind, m.Primary())
					continue
				}
				checked++
				near := d.CompareWith(m, perturb(d, m, insideFrac))
				if near.OK && near.Verdict == VerdictMismatch {
					t.Errorf("%s [%s]: a perturbation of %.0f%% of its own mismatch threshold "+
						"(drift=%g mismatch=%g%s) already reads MISMATCH (norm=%.2f, %q) -- "+
						"the thresholds are not on this measurement's scale",
						d.ID, d.Kind, insideFrac*100, drift, mismatch, unitSuffix(d), near.Norm, near.Display)
				}

				far := d.CompareWith(m, perturb(d, m, pastFrac))
				if far.OK && far.Verdict == VerdictOK {
					t.Errorf("%s [%s]: a perturbation of %.0fx its own mismatch threshold "+
						"(drift=%g mismatch=%g%s) still reads ok (norm=%.2f, %q) -- "+
						"the thresholds are far too loose to catch anything",
						d.ID, d.Kind, pastFrac, drift, mismatch, unitSuffix(d), far.Norm, far.Display)
				}
			}
		})
	}
	if checked < minCheckedDescriptors {
		t.Errorf("the scale gate only exercised %d (descriptor, probe) pairs, want >= %d -- "+
			"coverage collapsed, so a passing result proves nothing",
			checked, minCheckedDescriptors)
	}
	t.Logf("scale gate exercised %d (descriptor, probe) pairs", checked)
}

func unitSuffix(d Descriptor) string {
	if d.Unit == "" {
		return ""
	}
	return " " + d.Unit
}

// Norm ≥ 1 means mismatch for EVERY descriptor, which is what makes a cross-unit ranking meaningful.
func TestDescriptorNormIsComparableAcrossDescriptors(t *testing.T) {
	for _, probe := range scaleProbes() {
		b := NewBundle(probe.w, probe.name)
		for _, d := range DefaultRegistry().Applicable(b) {
			m := d.Extract(b)
			if !m.OK {
				continue
			}
			pm := perturb(d, m, 0.1)
			if !reaches(d, m, perturb(d, m, 3.0)) {
				continue
			}
			got := d.CompareWith(m, pm)
			if !got.OK {
				continue
			}
			if got.Norm >= 1 {
				t.Errorf("%s [%s/%s]: Norm=%.2f on a well-matched pair -- Norm>=1 must mean "+
					"'at or past mismatch', so this poisons any cross-descriptor ranking",
					d.ID, probe.name, m.Form, got.Norm)
			}
		}
	}
}
