package fingerprint

import (
	"fmt"
	"math"
)

// A descriptor declares WHAT it measures, Kind the scale, and the comparator what "different" means on it, which
// keeps the common case one declarative row while an exotic descriptor supplies its own Compare.

// Norm >= 1 means "at or past mismatch" for EVERY descriptor whatever its Kind or Form, which makes a cross-axis
// worst-offender ranking meaningful. A ranking aid ONLY, never summed into a composite.
type Delta struct {
	Norm   float64 // scale-normalized magnitude; >=1 means at/past mismatch
	Signed float64 // raw difference in the descriptor's own units (unsigned for vector/track reductions)

	// The UNSIGNED magnitude weighed against the thresholds, in the units they are DECLARED in. Signed is the raw
	// difference, and Norm × mismatch fails for a comparator that transforms its thresholds (band.t60 compares
	// mean|log2|), so calibration needs this exactly. MagOK is false where Norm is a sentinel.
	Mag   float64
	MagOK bool

	Verdict   Verdict
	Direction string // "high" | "low" | "" (ok or undefined)
	Display   string // "7.1x short", "+36%", "+0.341"
	OK        bool
	NA        string // why, when !OK
}

// Vectors reduce to mean |Δ| and tracks to RMS after length alignment, then classify on the KindFraction scale.
func CompareByKind(k Kind, ref, synth Measurement) Delta {
	drift, mismatch := k.Thresholds()
	return CompareWithThresholds(k, ref, synth, drift, mismatch)
}

// For descriptors whose Kind has no single natural scale.
func CompareWithThresholds(k Kind, ref, synth Measurement, drift, mismatch float64) Delta {
	// "We could not measure this" is categorically different from "this matches", so no verdict is asserted.
	if !ref.OK {
		return Delta{NA: ref.NA}
	}
	if !synth.OK {
		return Delta{NA: synth.NA}
	}
	// Same SHAPE first: synth.Scalar is structurally zero for a vector, and would give a confident verdict on a number
	// never measured.
	if ref.Form != synth.Form {
		return Delta{NA: fmt.Sprintf(
			"form mismatch: reference measured as %s, synth as %s -- the same descriptor "+
				"produced different shapes for the two signals, so there is nothing to compare",
			ref.Form, synth.Form)}
	}
	switch ref.Form {
	case FormVector:
		return compareReduced(reduceVector(ref.Values, synth.Values), "mean|Δ|", drift, mismatch)
	case FormTrack:
		return compareReduced(reduceTrack(ref.Values, synth.Values), "rms|Δ|", drift, mismatch)
	default:
		return compareScalar(k, ref.Scalar, synth.Scalar, drift, mismatch)
	}
}

// A ratio for durations, a signed percentage for frequencies, a signed delta otherwise.
func compareScalar(k Kind, ref, synth, drift, mismatch float64) Delta {
	d := Delta{Signed: synth - ref, OK: true}

	switch k {
	case KindSeconds, KindRatio:
		d.Verdict, d.Direction = classifySeconds(ref, synth, drift, mismatch)
		d.Norm, d.Display = ratioNormAndDisplay(k, ref, synth, mismatch)
		if ref > 0 && synth > 0 {
			folded := synth / ref
			if folded < 1 {
				folded = 1 / folded
			}
			d.Mag, d.MagOK = folded, true
		}

	case KindHz:
		d.Verdict, d.Direction = classifyHz(ref, synth, drift, mismatch)
		if ref <= 0 {
			d.Display = "n/a"
			// A zero reference makes the percentage undefined; the verdict already says "synth has energy where ref has none".
			if synth > 0 {
				d.Norm = 1
			}
		} else {
			pct := (synth - ref) / ref * 100
			d.Display = fmt.Sprintf("%+.0f%%", pct)
			d.Norm = math.Abs(pct) / mismatch
			d.Mag, d.MagOK = math.Abs(pct), true
		}

	case KindRate:
		// A signed rate is an absolute delta, never a percentage, which is meaningless across zero.
		d.Verdict, d.Direction = classifyFraction(ref, synth, drift, mismatch)
		d.Display = fmt.Sprintf("%+.4g", d.Signed)
		d.Norm = math.Abs(d.Signed) / mismatch
		d.Mag, d.MagOK = math.Abs(d.Signed), true

	default: // every absolute-delta kind
		d.Verdict, d.Direction = classifyFraction(ref, synth, drift, mismatch)
		d.Display = fmt.Sprintf("%+.3f", d.Signed)
		d.Norm = math.Abs(d.Signed) / mismatch
		d.Mag, d.MagOK = math.Abs(d.Signed), true
	}
	return d
}

// "n/a" for a non-positive value on either side.
func ratioNormAndDisplay(k Kind, ref, synth, mismatchRatio float64) (norm float64, display string) {
	if ref <= 0 || synth <= 0 {
		if ref <= 0 && synth <= 0 {
			return 0, "n/a"
		}
		// One side has no event at all: maximally divergent on this axis.
		return 1, "n/a"
	}
	ratio := synth / ref
	folded := ratio
	if folded < 1 {
		folded = 1 / folded
	}
	norm = folded / mismatchRatio
	if ratio < 1 {
		display = fmt.Sprintf("%.1fx short", 1/ratio)
	} else {
		display = fmt.Sprintf("%.1fx long", ratio)
	}
	if k == KindRatio {
		display = fmt.Sprintf("%.2fx", ratio)
	}
	return norm, display
}

// Direction is meaningless for a multi-component reduction, so it is left empty. The thresholds are the
// DESCRIPTOR'S, in the vector's own units (Hz, cents, fractions): KindFraction's defaults made every non-fraction
// vector permanently red. No overrides means the Kind's defaults.
func compareReduced(mag float64, label string, drift, mismatch float64) Delta {
	return reducedDeltaDisplay(mag, fmt.Sprintf("%s %.3f", label, mag), drift, mismatch)
}

// For a comparator that formats its own value: appending the magnitude would print the same quantity twice in two
// renderings.
func reducedDeltaDisplay(mag float64, display string, drift, mismatch float64) Delta {
	if mismatch <= 0 {
		drift, mismatch = KindFraction.Thresholds()
	}
	v := VerdictOK
	switch {
	case mag >= mismatch:
		v = VerdictMismatch
	case mag >= drift:
		v = VerdictDrift
	}
	return Delta{
		Norm:    mag / mismatch,
		Signed:  mag,
		Mag:     mag,
		MagOK:   true,
		Verdict: v,
		Display: display,
		OK:      true,
	}
}

// Only the overlapping prefix: zero-padding would manufacture divergence from an analysis-config length difference.
func reduceVector(a, b []float64) float64 {
	n := min(len(a), len(b))
	if n == 0 {
		return 0
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += math.Abs(a[i] - b[i])
	}
	return sum / float64(n)
}

// Nearest-neighbour resampled to the longer length: the same gesture at two hop rates must compare as similar.
func reduceTrack(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := max(len(a), len(b))
	sum := 0.0
	for i := 0; i < n; i++ {
		d := sampleAt(a, i, n) - sampleAt(b, i, n)
		sum += d * d
	}
	return math.Sqrt(sum / float64(n))
}

// src must be non-empty.
func sampleAt(src []float64, i, n int) float64 {
	if len(src) == n {
		return src[i]
	}
	idx := int(float64(i) * float64(len(src)) / float64(n))
	if idx >= len(src) {
		idx = len(src) - 1
	}
	return src[idx]
}
