package fingerprint

import (
	"math"
	"testing"
)

// Calibration writes Mag's percentiles back into Drift/Mismatch, so a Mag in the wrong unit silently makes the axis
// fire on everything or nothing. The invariant: classifying Mag against the descriptor's own thresholds reproduces
// the comparator's verdict.

func TestDeltaMagReproducesTheVerdict(t *testing.T) {
	bundles := map[string]*Bundle{}
	for _, p := range scaleProbes() {
		bundles[p.name] = NewBundle(p.w, p.name)
	}

	checked := 0
	for _, d := range DefaultRegistry().All() {
		drift, mismatch := d.Thresholds()
		for _, p := range scaleProbes() {
			b := bundles[p.name]
			if d.Applies != nil && !d.Applies(b) {
				continue
			}
			ref := d.Extract(b)
			if !ref.OK {
				continue
			}
			// A SWEEP: a comparator with Mag in the wrong domain still agrees by coincidence over part of the range.
			for _, frac := range []float64{0.2, 0.6, 1.0, 1.4, 1.8, 2.4, 3.0} {
				synth := perturb(d, ref, frac)
				got := d.CompareWith(ref, synth)
				if !got.OK || !got.MagOK {
					continue
				}
				if math.IsNaN(got.Mag) || math.IsInf(got.Mag, 0) {
					t.Errorf("%s/%s frac=%g: Mag=%v is not a number", d.ID, p.name, frac, got.Mag)
					continue
				}
				// frac=1 lands exactly on a threshold, where a transform round trip (2^log2 x ≠ x bit for bit) decides the tie, so both
				// sides of a one-ULP tie are accepted: the property is the UNITS, not how a tie breaks.
				strict, tolerant := verdictFor(got.Mag, drift, mismatch, 0), verdictFor(got.Mag, drift, mismatch, 1e-9)
				if got.Verdict != strict && got.Verdict != tolerant {
					t.Errorf("%s/%s frac=%g: Mag=%g against drift=%g mismatch=%g implies %s, "+
						"but the comparator returned %s — Mag is not in the declared threshold units",
						d.ID, p.name, frac, got.Mag, drift, mismatch, strict, got.Verdict)
				}
				checked++
			}
		}
	}
	if checked < 40 {
		t.Fatalf("only %d (descriptor, probe) pairs reported a usable Mag — the gate has gone hollow", checked)
	}
	t.Logf("verified Delta.Mag against declared thresholds on %d (descriptor, probe, frac) triples", checked)
}

// rel=0 is the strict rule; within rel of a threshold counts as reaching it.
func verdictFor(mag, drift, mismatch, rel float64) Verdict {
	at := func(v, th float64) bool {
		return v >= th || math.Abs(v-th) <= rel*math.Max(1, math.Abs(th))
	}
	switch {
	case at(mag, mismatch):
		return VerdictMismatch
	case at(mag, drift):
		return VerdictDrift
	default:
		return VerdictOK
	}
}

func TestDeltaMagUnitsPerKind(t *testing.T) {
	cases := []struct {
		name       string
		kind       Kind
		ref, synth float64
		wantMag    float64
	}{
		{"seconds compares a folded ratio", KindSeconds, 0.01, 0.03, 3},
		{"seconds folds the short direction too", KindSeconds, 0.03, 0.01, 3},
		{"ratio compares a folded ratio", KindRatio, 2, 1, 2},
		{"hz compares a percentage of the reference", KindHz, 1000, 1400, 40},
		{"hz folds the negative direction", KindHz, 1000, 600, 40},
		{"fraction compares an absolute delta", KindFraction, 0.2, 0.35, 0.15},
		{"dB compares an absolute delta", KindDB, -12, -6, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			drift, mismatch := c.kind.Thresholds()
			got := CompareWithThresholds(c.kind, Scalar(c.ref), Scalar(c.synth), drift, mismatch)
			if !got.MagOK {
				t.Fatalf("MagOK=false for a measurable pair (%v -> %v)", c.ref, c.synth)
			}
			if math.Abs(got.Mag-c.wantMag) > 1e-9 {
				t.Fatalf("Mag=%g, want %g", got.Mag, c.wantMag)
			}
		})
	}
}

// Norm=1 there is set by fiat, a statement rather than a measurement, so calibration must not average it.
func TestDeltaMagIsFalseForSentinelRows(t *testing.T) {
	drift, mismatch := KindSeconds.Thresholds()
	got := CompareWithThresholds(KindSeconds, Scalar(1.0), Scalar(0), drift, mismatch)
	if got.MagOK {
		t.Errorf("a zero-on-one-side seconds pair reported MagOK with Mag=%g", got.Mag)
	}
	dHz, mHz := KindHz.Thresholds()
	got = CompareWithThresholds(KindHz, Scalar(0), Scalar(440), dHz, mHz)
	if got.MagOK {
		t.Errorf("a zero-reference hz pair reported MagOK with Mag=%g", got.Mag)
	}
}
