package ui

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// TestBenchFmtMSRoundsToTwoDecimals exercises the human-readable
// benchmark formatter: seconds in, milliseconds with two decimals out.
// The format is part of the bench log contract that perf_e2e + the
// scripts/perf grep scrape — any change here can silently break the
// graphs in bench-results/.
func TestBenchFmtMSRoundsToTwoDecimals(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0.00"},
		{0.001, "1.00"},        // 1 ms
		{0.012345, "12.35"},     // round half up
		{0.99995, "999.95"},
		{1, "1000.00"},
		{-0.0005, "-0.50"},
	}
	for _, c := range cases {
		got := benchFmtMS(c.in)
		if got != c.want {
			t.Errorf("benchFmtMS(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestBenchFmtMSNaNInfReturnsNA pins the documented "N/A" sentinel for
// undefined inputs. Log readers depend on this token to skip undefined
// rows instead of inserting bogus zeros.
func TestBenchFmtMSNaNInfReturnsNA(t *testing.T) {
	for _, in := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := benchFmtMS(in); got != "N/A" {
			t.Errorf("benchFmtMS(%v) = %q, want %q", in, got, "N/A")
		}
	}
}

// TestBenchJsonMSRoundsToThreeDecimals — JSON variant uses three
// decimals (sub-millisecond resolution) because perf_e2e.json is
// machine-readable and downstream diffs are sensitive to noise.
func TestBenchJsonMSRoundsToThreeDecimals(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0.000"},
		{0.000123, "0.123"},
		{0.0123456, "12.346"},
		{1, "1000.000"},
	}
	for _, c := range cases {
		got := benchJsonMS(c.in)
		if got != c.want {
			t.Errorf("benchJsonMS(%v) = %q, want %q", c.in, got, c.want)
		}
		// Output must be a parseable float so JSON marshaling downstream
		// emits a number, not a string.
		if _, err := strconv.ParseFloat(got, 64); err != nil {
			t.Errorf("benchJsonMS(%v) = %q is not a number: %v", c.in, got, err)
		}
	}
}

// TestBenchJsonMSNaNInfReturnsNullLiteral pins the documented null
// fallback. Returning "N/A" here would emit invalid JSON downstream.
func TestBenchJsonMSNaNInfReturnsNullLiteral(t *testing.T) {
	for _, in := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		got := benchJsonMS(in)
		if got != "null" {
			t.Errorf("benchJsonMS(%v) = %q, want %q", in, got, "null")
		}
		// Real Go json.Marshal of a literal "null" round-trips; this
		// double-checks no stray whitespace / quotes crept in.
		if strings.ContainsAny(got, " \t\n\"") {
			t.Errorf("benchJsonMS(%v) = %q has stray chars", in, got)
		}
	}
}
