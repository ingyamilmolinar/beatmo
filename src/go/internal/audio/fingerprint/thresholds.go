package fingerprint

import (
	_ "embed"
	"encoding/json"
)

// thresholds.go — Drift/Mismatch values earned from the corpus.
//
// WHY THIS FILE EXISTS
// --------------------
// Every descriptor's drift/mismatch pair started life as a hand-guessed
// number, and the guesses SATURATED: `refs validate` showed MFCC profile
// firing on 98% of instruments and spectral irregularity on 100%. An axis
// that flags everything is a constant, not a measurement — it cannot rank
// anything, and it crowds the axes that carry signal off the top of the
// table.
//
// `refs calibrate-thresholds` replaces the guess with evidence. It takes the
// families the listener graded "y" as the operational definition of "close
// enough" and, per descriptor, sets
//
//	drift    = p50 of Delta.Mag over the good group
//	mismatch = p90 of Delta.Mag over the good group
//
// so an axis fires when an instrument is further from its reference than the
// instruments the listener accepted.
//
// UNITS — the part that is easy to get wrong. Delta.Mag is the magnitude the
// comparator actually weighed against drift and mismatch, in the units those
// thresholds are DECLARED in. That is deliberately NOT Delta.Signed: a
// KindSeconds or KindRatio axis is compared as a folded RATIO (>= 1 by
// construction) and a KindHz axis as a PERCENTAGE of the reference, so
// calibrating on the raw difference writes a number in the wrong scale — a
// sub-1 "ratio" that fires on everything, or a percentage in the hundreds
// that fires on nothing. It is also not Norm x mismatch, which reconstructs
// Mag correctly for every default comparator but not for one that transforms
// its thresholds into a working domain (band.t60 declares T60 ratios and
// compares mean|log2 ratio|). The comparator that owns the transform reports
// Mag itself; fingerprint's TestDeltaMagReproducesTheVerdict gates that it is
// honest for every descriptor.
//
// The values are therefore per DESCRIPTOR and not per Kind — KindRate alone
// spans Hz/sec, dB/sec and units/sec², orders of magnitude apart.
//
// PRECEDENCE. A "probe" entry (a listener-measured audibility step from
// `refs probe`, i.e. the smallest change a human could actually hear) beats a
// corpus entry: an ear beats a percentile. A "hand" entry is a deliberate
// EXCLUSION from calibration, written when the calibrated value stopped
// `cmd/refs`'s acceptance gate reproducing a listener complaint — a
// calibration that makes the tool disagree with the ear is wrong by
// definition, whatever the corpus says. Both survive recalibration untouched,
// and both should carry a Note saying why they were pinned; an unexplained
// pin is indistinguishable from a stale one.
//
// The file is EMBEDDED, so regenerating it requires a rebuild before the new
// values take effect.

//go:embed testdata/thresholds.json
var thresholdJSON []byte

// ThresholdEntry is one descriptor's calibrated drift/mismatch pair.
//
// N records how many good-group samples the percentiles were taken over, so a
// reader can tell a value earned from 12 families from one scraped out of 5.
// Source is "corpus", "probe" or "hand" (see the precedence note above).
// Note carries the REASON a probe/hand entry was pinned — the evidence that
// justifies overriding the corpus, without which a reviewer cannot tell a
// deliberate exclusion from a leftover.
type ThresholdEntry struct {
	Drift    float64 `json:"drift"`
	Mismatch float64 `json:"mismatch"`
	N        int     `json:"n"`
	Source   string  `json:"source"`
	Note     string  `json:"note,omitempty"`
}

// ThresholdFile maps descriptor ID to its calibrated thresholds.
type ThresholdFile map[string]ThresholdEntry

// LoadThresholdFile decodes the embedded calibration.
//
// A malformed file yields an EMPTY map rather than an error: the hand
// thresholds on the descriptors are a working fallback, and refusing to build
// the registry because a calibration artifact is corrupt would take the whole
// tool down over an optimisation.
func LoadThresholdFile() ThresholdFile {
	out := ThresholdFile{}
	if len(thresholdJSON) > 0 {
		_ = json.Unmarshal(thresholdJSON, &out)
	}
	return out
}

// Percentile is the package's interpolating percentile (p in 0..100), exposed
// for `refs calibrate-thresholds`.
//
// Interpolating rather than index-rounding because the good group is small: at
// n=8 the rounded p90 index is simply the maximum, so a single outlier family
// would BE the mismatch threshold for every axis. Interpolation at least
// blends the top two.
func Percentile(sorted []float64, p float64) float64 { return percentile(sorted, p) }
