package fingerprint

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// registry_scale_test.go — every descriptor's thresholds must be on the same
// scale as the quantity it measures.
//
// THE GAP THIS CLOSES
// -------------------
// cmd/refs/diagnose_acceptance_test.go replays a FROZEN JSON baseline covering
// the ten original hand-written axes. It is a regression record, not a gate on
// new descriptors: the twenty-nine added by registry_family.go and
// registry_tracks.go were never exercised end to end at all, and two of them
// shipped permanently red as a result. `wind.attack_centroid_traj` reduced a
// vector of HERTZ and classified the result against KindFraction's 0.05/0.15,
// so two trajectories 45 Hz apart -- a match by any musical standard -- scored
// MISMATCH with Norm=300, and its declared Drift/Mismatch were discarded
// entirely.
//
// This test needs no reference audio and no frozen data. It asks each
// descriptor two questions in its OWN units:
//
//   - a perturbation well INSIDE its mismatch threshold must read ok
//   - a perturbation well PAST it must not read ok
//
// A descriptor whose thresholds are in the wrong units fails both.

// scaleProbes are signals chosen so that between them every applicability
// predicate in the registry fires: a sustained pitched tone, a percussive hit
// with no sustain plateau, and a noisy pitched tone.
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

// perturb shifts m by frac times d's own mismatch threshold, in d's own units.
//
// KindSeconds and KindRatio compare as a RATIO of larger to smaller, so their
// threshold is a multiplier and the perturbation has to be multiplicative too;
// every other kind compares as an absolute delta. Getting this split right is
// the whole reason the helper exists rather than a bare "+= 0.1".
func perturb(d Descriptor, m Measurement, frac float64) Measurement {
	_, mismatch := d.Thresholds()
	out := m
	switch d.Kind {
	case KindSeconds, KindRatio:
		// ratio 1 + frac*(mismatch-1): frac=0 leaves it alone, frac=1 lands
		// exactly on the mismatch ratio.
		scale := 1 + frac*(mismatch-1)
		out.Scalar = m.Scalar * scale
		if len(m.Values) > 0 {
			out.Values = append([]float64(nil), m.Values...)
			if d.Scope == ScopeBand {
				// A ScopeBand vector (e.g. band.t60) has no "headline"
				// component -- every band is an independent peer, and its
				// Compare reduces by MEAN across them. Moving only index 0
				// would get diluted by every other untouched band and could
				// never reach a mean-based comparator; a uniform per-band
				// ratio shift is what "every band moved by the same amount"
				// actually looks like, and is what the descriptor needs to
				// probe its own drift/mismatch boundary.
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

// shiftAll offsets a vector/track payload. A COMPOUND measurement's later
// components are flags and fallbacks (ring T60's extrapolation bit), so only
// the headline value -- the one Primary() reports -- may be moved.
//
// This used to key on "has a custom comparator", which was a proxy for
// "compound" and stopped being true once genuine all-peer vectors
// (mode.ratios, decay.per_mode) grew custom comparators: their perturbation
// silently shrank to a one-component nudge that a mean-based reduction
// dilutes away, so the gate stopped testing them. Descriptor.Compound says
// what is actually meant.
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

// reaches reports whether a perturbation actually changed what the
// comparator consumes.
//
// It legitimately does not in two cases, and neither is a product defect:
// a probe signal that measures exactly 0 on some axis cannot be perturbed
// MULTIPLICATIVELY (which is how ratio-scaled kinds must move), and a compound
// measurement whose custom comparator escalated to a component the helper is
// not allowed to touch (ring T60 falling back to decay RATE when both sides
// clamp) is unmoved by shifting its headline value. Skipping those keeps the
// assertion honest; minCheckedDescriptors below stops the skip from quietly
// swallowing the whole battery.
func reaches(d Descriptor, m, perturbed Measurement) bool {
	base := d.CompareWith(m, m)
	moved := d.CompareWith(m, perturbed)
	return base.Display != moved.Display || base.Norm != moved.Norm || base.Verdict != moved.Verdict
}

// minCheckedDescriptors is the floor on how many (descriptor, probe) pairs the
// scale gate must actually exercise. Without it, a change that made every
// perturbation a no-op would turn this test green while checking nothing.
const minCheckedDescriptors = 40

// TestDescriptorThresholdsAreOnTheRightScale is the gate.
func TestDescriptorThresholdsAreOnTheRightScale(t *testing.T) {
	// insideFrac must read ok; pastFrac must not. The gap between them leaves
	// room for a comparator that folds or clamps near the boundary.
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

// TestDescriptorNormIsComparableAcrossDescriptors pins Delta.Norm's documented
// contract: Norm >= 1 means "at or past mismatch" for EVERY descriptor
// whatever its kind or form. That is the only thing making a worst-offending
// ranking meaningful across seconds, hertz, decibels and unit fractions -- and
// it was silently false for the vector descriptors, whose Norm ran to 300 on a
// matching pair.
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
