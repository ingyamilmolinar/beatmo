package fingerprint

// Kind selects both the default comparator and the thresholds, lifted verbatim from the calibrated diagnose values so
// the acceptance test keeps passing.

// Kind classifies a descriptor's natural scale.
type Kind int

const (
	// An absolute delta: 0.05 is already audible, 0.15 night and day.
	KindFraction Kind = iota

	// An order of magnitude smaller: inharmonicity is <0.001 for a clean tone and 0.005–0.03 for a struck bar.
	KindTinyFraction

	// A ratio of larger to smaller ("7x short"): 1.3x is segmentation noise, 2x unambiguous.
	KindSeconds

	// A signed percentage of the reference.
	KindHz

	// 0 for a clean consecutive series, ~0.2–0.6 for a bar or tube whose overtones skip ranks, several units for a noisy
	// source.
	KindSeriesDeviation

	// KindDB is a level or level difference in decibels.
	KindDB

	KindRatio

	// SIGNED: a rate crosses zero, so a percentage of the reference is meaningless (and explodes near zero). A signed
	// absolute delta, with per-descriptor thresholds supplying the scale.
	KindRate
)

// Ratios for KindSeconds/KindRatio, percentages for KindHz, absolute deltas in the kind's units otherwise.
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
		// A rate descriptor sets its own; a default that flagged everything would be worse than one that flags nothing until
		// calibrated.
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

// A closed set, not a score: collapsing axes into one weighted scalar let a 7x decay error cancel a small spectral gain.
type Verdict string

const (
	VerdictOK       Verdict = "ok"
	VerdictDrift    Verdict = "drift"
	VerdictMismatch Verdict = "MISMATCH"
)

// The only ordering on verdicts: never arithmetic, never summed.
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
