package ui

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// The bench log format is scraped by perf_e2e and scripts/perf; a change silently breaks their graphs.
func TestBenchFmtMSRoundsToTwoDecimals(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0.00"},
		{0.001, "1.00"},     // 1 ms
		{0.012345, "12.35"}, // round half up
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

// Log readers skip "N/A" rows; a zero would be read as a measurement.
func TestBenchFmtMSNaNInfReturnsNA(t *testing.T) {
	for _, in := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := benchFmtMS(in); got != "N/A" {
			t.Errorf("benchFmtMS(%v) = %q, want %q", in, got, "N/A")
		}
	}
}

// Three decimals: perf_e2e.json is machine-read and its diffs are sensitive to noise.
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
		// Parseable, so the JSON carries a number, not a string.
		if _, err := strconv.ParseFloat(got, 64); err != nil {
			t.Errorf("benchJsonMS(%v) = %q is not a number: %v", c.in, got, err)
		}
	}
}

// "N/A" here would be invalid JSON.
func TestBenchJsonMSNaNInfReturnsNullLiteral(t *testing.T) {
	for _, in := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		got := benchJsonMS(in)
		if got != "null" {
			t.Errorf("benchJsonMS(%v) = %q, want %q", in, got, "null")
		}
		if strings.ContainsAny(got, " \t\n\"") {
			t.Errorf("benchJsonMS(%v) = %q has stray chars", in, got)
		}
	}
}
