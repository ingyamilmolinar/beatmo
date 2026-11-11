package fingerprint

import (
	"fmt"
	"math"
)

// compare.go — turning two Measurements into a Delta.
//
// Measurement and comparison are deliberately separate concerns. A descriptor
// declares WHAT it measures; Kind declares the scale it lives on; the
// comparator here decides what "different" means on that scale. That
// separation is what keeps the common case to a single declarative registry
// row with no code, while still letting an exotic descriptor supply its own
// Compare (the ring-T60 escalation being the first real case).

// Delta is the comparison of one descriptor between reference and synth.
//
// Norm is the cross-descriptor currency: it is scaled so that Norm >= 1 means
// "at or past this descriptor's mismatch threshold" for EVERY descriptor,
// whatever its Kind or Form. That is what makes a worst-offending-axes
// ranking meaningful across a battery whose members are measured in seconds,
// hertz, decibels and unitless fractions. It is a ranking aid ONLY — it is
// never summed across axes into a composite, which is the failure mode
// documented in 2026-08-22-verdict-to-metric-mapping.md.
type Delta struct {
	Norm   float64 // scale-normalized magnitude; >=1 means at/past mismatch
	Signed float64 // raw difference in the descriptor's own units (unsigned for vector/track reductions)

	// Mag is the UNSIGNED magnitude this comparator weighed against drift and
	// mismatch, expressed in the units those thresholds are DECLARED in, and
	// MagOK says whether it means anything.
	//
	// It exists because that quantity is not recoverable from the other
	// fields. Signed is the raw difference, which is only what gets compared
	// for the absolute-delta kinds -- a KindSeconds axis is compared as a
	// folded RATIO and a KindHz axis as a PERCENTAGE. Norm/mismatch looks
	// like it recovers it, and does for every default path, but silently
	// fails for a comparator that TRANSFORMS its thresholds: band.t60
	// declares ratios and compares mean|log2|, so Norm x mismatch there is
	// neither a ratio nor a log2. Threshold calibration
	// (cmd/refs/calibrate_thresholds.go) needs the declared-unit magnitude
	// exactly, so the comparator that knows the transform reports it rather
	// than a downstream reader guessing.
	//
	// MagOK is false for the degenerate branches ("one side had no event at
	// all") where Norm is a sentinel rather than a measurement.
	Mag   float64
	MagOK bool

	Verdict   Verdict
	Direction string // "high" | "low" | "" (ok or undefined)
	Display   string // "7.1x short", "+36%", "+0.341"
	OK        bool
	NA        string // why, when !OK
}

// CompareByKind is the default comparator for k.
//
// Scalars dispatch to the classifier matching the kind. Vectors reduce to
// mean absolute difference and tracks to RMS difference after length
// alignment; both are already normalized magnitudes, so they are then
// classified on the KindFraction scale regardless of the declared kind.
func CompareByKind(k Kind, ref, synth Measurement) Delta {
	drift, mismatch := k.Thresholds()
	return CompareWithThresholds(k, ref, synth, drift, mismatch)
}

// CompareWithThresholds is CompareByKind with explicit thresholds, for
// descriptors whose Kind has no single natural scale (see Descriptor.Drift).
func CompareWithThresholds(k Kind, ref, synth Measurement, drift, mismatch float64) Delta {
	// An unavailable measurement on either side propagates its reason. No
	// verdict is asserted: "we could not measure this" is categorically
	// different from "this matches".
	if !ref.OK {
		return Delta{NA: ref.NA}
	}
	if !synth.OK {
		return Delta{NA: synth.NA}
	}
	// The two sides must be the same SHAPE before any of them can be
	// compared. Dispatching on ref.Form alone read synth.Scalar -- which is
	// structurally zero for a vector -- and returned a confident verdict from
	// a number that was never measured.
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

// compareScalar classifies a scalar pair on k's scale and renders it the way
// cmd/refs/diagnose.go's formatDelta did: a ratio for durations, a signed
// percentage for frequencies, a signed absolute delta otherwise.
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
			// A zero reference makes the percentage undefined; the verdict
			// above already encodes "synth has energy where ref has none".
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
		// A signed rate: absolute delta, never a percentage. Rendered with
		// enough precision to read a sign reversal (+57 -> -516) rather than
		// a ratio that would be meaningless across zero.
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

// ratioNormAndDisplay renders a duration/ratio comparison. Mirrors
// formatDelta's seconds branch exactly, including its "n/a" for a
// non-positive value on either side.
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

// compareReduced classifies an already-reduced, unsigned magnitude (a vector
// or track difference). Direction is meaningless for a multi-component
// reduction — "the profile differs" has no single sign — so it is deliberately
// left empty rather than invented.
//
// The thresholds are the DESCRIPTOR'S, in the vector's own units, because a
// reduction is a magnitude in those units: mean|Δ| over a centroid trajectory
// is hertz, over a piano stretch curve is cents, over a band-energy profile is
// a unit fraction. Hardcoding KindFraction's 0.05/0.15 here made every
// non-fraction vector permanently red -- two centroid trajectories 45 Hz apart
// scored MISMATCH with Norm=300 -- and silently discarded the Drift/Mismatch
// overrides Descriptor advertises for exactly this case.
//
// A descriptor that declares no overrides still lands on its Kind's defaults,
// so unit-fraction profiles behave exactly as before.
func compareReduced(mag float64, label string, drift, mismatch float64) Delta {
	return reducedDeltaDisplay(mag, fmt.Sprintf("%s %.3f", label, mag), drift, mismatch)
}

// reducedDeltaDisplay is compareReduced for a comparator that has ALREADY
// formatted its own display string, value included.
//
// compareReduced appends "%.3f" of the magnitude to whatever label it is
// given, which is right for the generic "mean|Δ|" / "rms|Δ|" reductions whose
// label is a bare operator name. It is wrong for a comparator that formats
// the quantity itself, in its own units and its own precision: band.t60's
// "mean 2.43x (worst 4k 6.52x)" came out as
// "mean 2.43x (worst 4k 6.52x) 1.280", printing two different renderings of
// the same number side by side and inviting the reader to treat the trailing
// one as a separate quantity. Such a comparator passes its finished string
// here and it is used verbatim.
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

// reduceVector returns the mean absolute difference over the overlapping
// prefix of a and b. Comparing only the overlap (rather than zero-padding)
// avoids manufacturing divergence from a length difference that is an
// analysis-configuration artifact rather than a property of the signals.
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

// reduceTrack returns the RMS difference between two time series after
// resampling the shorter onto the longer's length by nearest neighbour. Two
// recordings of the same gesture at different hop rates must compare as
// similar, not as a length mismatch.
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

// sampleAt reads src as if it had been resampled to length n, by nearest
// neighbour. src must be non-empty.
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
