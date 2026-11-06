package fingerprint

// kind.go — the natural scale of a descriptor, and the three-way verdict.
//
// Kind selects BOTH the default comparator (compare.go) and the
// drift/mismatch thresholds below. Every threshold value here is lifted
// verbatim from cmd/refs/diagnose.go, where it was calibrated against the
// worked examples in
// docs/superpowers/specs/2026-08-22-verdict-to-metric-mapping.md. Moving them
// without changing them is what lets the diagnose acceptance test keep
// passing across the registry refactor.

// Kind classifies a descriptor's natural scale.
type Kind int

const (
	// KindFraction is a unitless fraction spanning most of (0..1) — noise
	// fraction, spectral flatness, sustain modulation. Compared as an
	// absolute delta: 0.05 is already an audible shift (noise fraction
	// 0.06 -> 0.11) and 0.15 is night-and-day.
	KindFraction Kind = iota

	// KindTinyFraction is a fraction whose natural scale is an order of
	// magnitude smaller than KindFraction. Inharmonicity is <0.001 for a
	// clean string/wind tone and 0.005-0.03 for a genuinely inharmonic
	// struck bar or tube, so it needs its own, tighter thresholds.
	KindTinyFraction

	// KindSeconds is a duration, compared as a ratio of larger to smaller:
	// "7x short" is the natural unit for a timing error (the harp case,
	// 1.130s reference vs 0.160s synth). 1.3x is within ordinary
	// segmentation noise; 2x is unambiguous.
	KindSeconds

	// KindHz is a frequency, compared as a signed percentage of the
	// reference. Calibrated so the mapping doc's +36% worked example lands
	// on drift rather than mismatch.
	KindHz

	// KindSeriesDeviation is PartialRatios.SeriesDeviation, which runs on
	// its own scale again: 0 for a clean consecutive harmonic series,
	// roughly 0.2-0.6 for a tuned bar or tube whose overtones skip ranks
	// (kalimba and tubular-bells measure 0.4-0.6 in the corpus), and into
	// several units for a genuinely noisy source where F0 itself is
	// unreliable.
	KindSeriesDeviation

	// KindDB is a level or level difference in decibels.
	KindDB

	// KindRatio is a dimensionless ratio compared the same way as
	// KindSeconds, but rendered without duration semantics.
	KindRatio

	// KindRate is a SIGNED rate of change: Hz/sec, dB/sec, units/sec^2. It
	// must not be compared as a percentage of the reference (KindHz) because
	// a rate crosses zero: a reference of +57 Hz/s against a synth of
	// -516 Hz/s is a complete reversal of direction, but "-1013%" is a
	// meaningless rendering of it, and a reference near zero makes the
	// percentage explode or go undefined. Compared as a signed absolute delta
	// instead, with the per-descriptor Drift/Mismatch overrides supplying the
	// scale, since Hz/sec and 1/sec live orders of magnitude apart.
	KindRate
)

// Thresholds returns the (drift, mismatch) pair for k.
//
// For KindSeconds and KindRatio these are RATIOS of larger-to-smaller. For
// KindHz they are percentages. For every other kind they are absolute deltas
// in the kind's own units.
func (k Kind) Thresholds() (drift, mismatch float64) {
	switch k {
	case KindTinyFraction:
		return 0.005, 0.02
	case KindSeriesDeviation:
		return 0.15, 0.5
	case KindSeconds, KindRatio:
		return 1.3, 2.0
	case KindHz:
		return 10.0, 40.0
	case KindDB:
		return 1.5, 6.0
	case KindRate:
		// Deliberately wide defaults: a rate descriptor is expected to set its
		// own Drift/Mismatch, and a default that silently flagged everything
		// would be worse than one that flags nothing until calibrated.
		return 1e9, 1e9
	default: // KindFraction
		return 0.05, 0.15
	}
}

// String renders k for test failure messages and JSON debugging.
func (k Kind) String() string {
	switch k {
	case KindTinyFraction:
		return "tiny-fraction"
	case KindSeconds:
		return "seconds"
	case KindHz:
		return "hz"
	case KindSeriesDeviation:
		return "series-deviation"
	case KindDB:
		return "dB"
	case KindRatio:
		return "ratio"
	case KindRate:
		return "rate"
	default:
		return "fraction"
	}
}

// Verdict is the three-way per-axis classification. Deliberately a closed set
// of three rather than a numeric score: the mapping doc establishes that
// collapsing axes into one weighted scalar is precisely the mechanism that
// let a 7x decay-time error cancel against a small spectral improvement, so
// winds scored +17..+42% better while sounding worse.
type Verdict string

const (
	VerdictOK       Verdict = "ok"
	VerdictDrift    Verdict = "drift"
	VerdictMismatch Verdict = "MISMATCH"
)

// severity ranks verdicts so the report can sort a "worst offending axes"
// list. This ranking is the ONLY ordering imposed on verdicts; it is
// deliberately not arithmetic and is never summed across axes.
func (v Verdict) severity() int {
	switch v {
	case VerdictMismatch:
		return 2
	case VerdictDrift:
		return 1
	default:
		return 0
	}
}

// WorseThan reports whether v is strictly more severe than other.
func (v Verdict) WorseThan(other Verdict) bool { return v.severity() > other.severity() }
