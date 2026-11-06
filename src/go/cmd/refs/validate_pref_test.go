//go:build !js

package main

import (
	"math"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestSignTestP pins signTestP's exact two-sided binomial values against
// hand-checked cases: 7/7 and 6/7 are asymmetric enough for one tail to
// dominate (doubling it directly), while 4/8 sits exactly on the
// distribution's centre, where both tails are equal and the doubled value
// exceeds 1 and must clamp.
func TestSignTestP(t *testing.T) {
	cases := []struct {
		agree, total int
		want         float64
	}{
		{7, 7, 0.0156},
		{6, 7, 0.125},
		{4, 8, 1.0},
	}
	for _, c := range cases {
		got := signTestP(c.agree, c.total)
		if math.Abs(got-c.want) > 1e-3 {
			t.Errorf("signTestP(%d,%d) = %v, want %v", c.agree, c.total, got, c.want)
		}
	}
}

// TestScorePreferencesFromNorms drives the pure counting core directly with
// precomputed per-clip norm maps -- no rendering, no diagnosis -- so the
// agreement rule itself (chosen's Norm strictly lower than rejected's) is
// exercised in isolation.
func TestScorePreferencesFromNorms(t *testing.T) {
	pairs := []prefPair{
		{Family: roster.Oboe, RefID: "oboe-A4"},
		{Family: roster.Oboe, RefID: "oboe-A4"},
		{Family: roster.Flute, RefID: "flute-A4"},
	}
	chosen := []map[string]float64{
		{"axis.a": 0.2, "axis.b": 1.5}, // pair 0: chosen lower on both
		{"axis.a": 0.9, "axis.b": 0.4}, // pair 1: chosen higher on a, lower on b
		nil,                            // pair 2: chosen undiagnosable -- skipped entirely
	}
	rejected := []map[string]float64{
		{"axis.a": 0.8, "axis.b": 2.0},
		{"axis.a": 0.3, "axis.b": 0.9},
		{"axis.a": 0.1},
	}
	got := scorePreferencesFromNorms(pairs, chosen, rejected)

	if e := got["axis.a"]; e != [2]int{1, 2} {
		t.Fatalf("axis.a = %v, want {agree:1 total:2} (pair 0 agrees, pair 1 does not, pair 2 skipped)", e)
	}
	if e := got["axis.b"]; e != [2]int{2, 2} {
		t.Fatalf("axis.b = %v, want {agree:2 total:2} (both measured pairs agree)", e)
	}
	if _, ok := got["axis.c"]; ok {
		t.Fatal("axis.c was never present in any chosen map and must not appear")
	}
}

// TestScorePreferencesFromNorms_MissingAxisOnRejected: a descriptor present
// only on the chosen clip's map (e.g. RefNA/SynthNA suppressed it on the
// rejected clip) must not count -- both sides have to be measurable for the
// comparison to mean anything.
func TestScorePreferencesFromNorms_MissingAxisOnRejected(t *testing.T) {
	pairs := []prefPair{{Family: "x", RefID: "x-A4"}}
	chosen := []map[string]float64{{"axis.only.chosen": 0.1}}
	rejected := []map[string]float64{{}}
	got := scorePreferencesFromNorms(pairs, chosen, rejected)
	if len(got) != 0 {
		t.Fatalf("want no rows (axis absent on rejected side), got %v", got)
	}
}

// TestMergePreferenceScores checks the ID-keyed merge in isolation: an axis
// present in prefByID gets its tally copied on; an axis absent is left at
// its zero value (which the PREF column renders as "-").
func TestMergePreferenceScores(t *testing.T) {
	scores := []axisScore{
		{ID: "axis.a", Axis: "a"},
		{ID: "axis.b", Axis: "b"},
	}
	prefByID := map[string][2]int{"axis.a": {5, 8}}
	got := mergePreferenceScores(scores, prefByID)

	if got[0].PrefAgree != 5 || got[0].PrefTotal != 8 {
		t.Fatalf("axis.a PrefAgree/PrefTotal = %d/%d, want 5/8", got[0].PrefAgree, got[0].PrefTotal)
	}
	if got[1].PrefAgree != 0 || got[1].PrefTotal != 0 {
		t.Fatalf("axis.b (absent from prefByID) PrefAgree/PrefTotal = %d/%d, want 0/0", got[1].PrefAgree, got[1].PrefTotal)
	}
}

// TestAxisFlagAppendsPredictivePref exercises the three boundary cases the
// controller specified directly: enough pairs and a high enough rate labels
// the axis; a lower rate on the same sample size does not; and a rate of 1.0
// with too few pairs still does not, because prefPredictiveMinTotal (6) is a
// floor on its own regardless of the rate.
func TestAxisFlagAppendsPredictivePref(t *testing.T) {
	cases := []struct {
		name         string
		agree, total int
		want         bool
	}{
		{"5 of 6 clears both bars", 5, 6, true},
		{"4 of 6 misses the rate bar", 4, 6, false},
		{"5 of 5 misses the sample-size floor despite a perfect rate", 5, 5, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := axisScore{AUC: 0.5, SE: 0.1, Comparable: true, PrefAgree: c.agree, PrefTotal: c.total}
			got := strings.Contains(axisFlag(s), "PREDICTIVE (pref)")
			if got != c.want {
				t.Fatalf("axisFlag(%d/%d) contains PREDICTIVE (pref) = %v, want %v (flag=%q)",
					c.agree, c.total, got, c.want, axisFlag(s))
			}
		})
	}
}

// TestScoreCandidateScales drives the pure per-candidate counting core
// directly against precomputed axis directions -- no diagnosis, no
// rendering -- covering the sign-agreement rule: a positive answer agrees
// with a "high" direction, a negative answer agrees with "low", disagreement
// is BACKWARDS, an axis absent or not measurable reports n/a and does not
// count toward Total, and Answer==0 is skipped entirely (no hit at all).
func TestScoreCandidateScales(t *testing.T) {
	scales := []sessionScale{
		{Q: "brighter?", Axis: "axis.bright", Answer: 2},    // agrees (high/high)
		{Q: "darker?", Axis: "axis.bright", Answer: -1},     // disagrees (low vs high) -- BACKWARDS
		{Q: "breathy?", Axis: "axis.noise", Answer: -1},     // agrees (low/low)
		{Q: "unmeasured?", Axis: "axis.missing", Answer: 1}, // no direction entry at all -- n/a
		{Q: "unreliable?", Axis: "axis.na", Answer: 1},      // present but not measurable -- n/a
		{Q: "no opinion", Axis: "axis.bright", Answer: 0},   // must be skipped: no hit, no tally
	}
	directions := map[string]scaleDirection{
		"axis.bright": {Direction: "high", Measurable: true},
		"axis.noise":  {Direction: "low", Measurable: true},
		"axis.na":     {Direction: "high", Measurable: false},
	}

	byAxis := map[string]*sessionScaleScore{}
	var order []string
	scoreCandidateScales(roster.Oboe, "A", scales, directions, byAxis, &order)

	bright := byAxis["axis.bright"]
	if bright == nil {
		t.Fatal("axis.bright missing from byAxis")
	}
	if bright.Agree != 1 || bright.Total != 2 {
		t.Fatalf("axis.bright agree/total = %d/%d, want 1/2 (one agree, one BACKWARDS)", bright.Agree, bright.Total)
	}
	if len(bright.Hits) != 2 {
		t.Fatalf("axis.bright hits = %d, want 2 (the Answer==0 row must be skipped entirely)", len(bright.Hits))
	}

	noise := byAxis["axis.noise"]
	if noise == nil || noise.Agree != 1 || noise.Total != 1 {
		t.Fatalf("axis.noise = %+v, want agree=1 total=1", noise)
	}

	missing := byAxis["axis.missing"]
	if missing == nil || missing.Total != 0 || len(missing.Hits) != 1 || missing.Hits[0].GotDir != "n/a" {
		t.Fatalf("axis.missing = %+v, want total=0, one n/a hit", missing)
	}

	na := byAxis["axis.na"]
	if na == nil || na.Total != 0 || len(na.Hits) != 1 || na.Hits[0].GotDir != "n/a" {
		t.Fatalf("axis.na = %+v, want total=0, one n/a hit (not measurable)", na)
	}

	wantOrder := []string{"axis.bright", "axis.noise", "axis.missing", "axis.na"}
	if len(order) != len(wantOrder) {
		t.Fatalf("order = %v, want %v", order, wantOrder)
	}
	for i, id := range wantOrder {
		if order[i] != id {
			t.Fatalf("order[%d] = %q, want %q", i, order[i], id)
		}
	}
}

// TestPrefCell_MarksSignificantlyBackwardsAxes: a small AGREE/TOTAL with a
// small p is not a weak result, it is a strong one pointing the wrong way.
// Reading it as weak is the mistake that let bright.centroid (AUC 0.36) look
// reasonable for as long as it did.
func TestPrefCell_MarksSignificantlyBackwardsAxes(t *testing.T) {
	for _, c := range []struct {
		name          string
		agree, total  int
		wantBackwards bool
	}{
		{"no pairs", 0, 0, false},
		{"significantly backwards", 1, 12, true},
		{"backwards but not significant", 1, 3, false},
		{"even split", 5, 10, false},
		{"agreeing", 11, 12, false},
	} {
		got := prefCell(axisScore{PrefAgree: c.agree, PrefTotal: c.total})
		if hasMark := strings.Contains(got, "backwards"); hasMark != c.wantBackwards {
			t.Errorf("%s (%d/%d): cell = %q, backwards marker = %v, want %v",
				c.name, c.agree, c.total, got, hasMark, c.wantBackwards)
		}
		if c.total == 0 && got != "-" {
			t.Errorf("%s: cell = %q, want \"-\"", c.name, got)
		}
	}
}

// An empty group has no mean. mean() reports NaN; the column must not print
// the literal "NaN", which reads as a computation that went wrong rather than
// as "there was nothing to average".
func TestMeanCell_EmptyGroupPrintsADash(t *testing.T) {
	if got := meanCell(mean(nil)); strings.Contains(got, "NaN") {
		t.Errorf("empty group rendered as %q", got)
	} else if strings.TrimSpace(got) != "-" {
		t.Errorf("empty group rendered as %q, want a dash", got)
	}
	if got := meanCell(1.5); got != "1.50" {
		t.Errorf("meanCell(1.5) = %q", got)
	}
}
