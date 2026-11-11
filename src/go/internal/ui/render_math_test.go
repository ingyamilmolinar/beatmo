//go:build test

package ui

import (
	"math"
	"testing"
)

func TestDBFromLinear(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{1.0, 0.0},
		{0.5, 20 * math.Log10(0.5)},
		{0.1, -20.0},
		{2.0, 20 * math.Log10(2)},
	}
	for _, tc := range cases {
		got := dBFromLinear(tc.in)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("dBFromLinear(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
	if got := dBFromLinear(0); !math.IsInf(got, -1) {
		t.Errorf("dBFromLinear(0) = %v, want -Inf", got)
	}
	if got := dBFromLinear(-0.5); !math.IsInf(got, -1) {
		t.Errorf("dBFromLinear(-0.5) = %v, want -Inf", got)
	}
}

func TestClampDBDisplay(t *testing.T) {
	if got := clampDBDisplay(math.Inf(-1)); got != -96.0 {
		t.Errorf("clampDBDisplay(-Inf) = %v, want -96", got)
	}
	for _, v := range []float64{0, -10, -50, -95.9, -200} {
		if got := clampDBDisplay(v); got != v {
			t.Errorf("clampDBDisplay(%v) = %v, want %v", v, got, v)
		}
	}
}

func TestFormatWindowMs(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0.5, "0.50ms"},
		{0.99, "0.99ms"},
		{1.0, "1.0ms"},
		{5.0, "5.0ms"},
		{9.99, "10.0ms"}, // boundary: 9.99 rounds-up via %.1f to "10.0"
		{10.0, "10ms"},
		{42.0, "42ms"},
		{1500.0, "1500ms"},
	}
	for _, tc := range cases {
		if got := formatWindowMs(tc.in); got != tc.want {
			t.Errorf("formatWindowMs(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDBToFrac(t *testing.T) {
	if got := dbToFrac(meterDBFloor); got != 0 {
		t.Errorf("dbToFrac(floor) = %v, want 0", got)
	}
	if got := dbToFrac(meterDBFloor - 1); got != 0 {
		t.Errorf("dbToFrac(below floor) = %v, want 0", got)
	}
	if got := dbToFrac(meterDBCeil); got != 1 {
		t.Errorf("dbToFrac(ceil) = %v, want 1", got)
	}
	if got := dbToFrac(meterDBCeil + 1); got != 1 {
		t.Errorf("dbToFrac(above ceil) = %v, want 1", got)
	}
	mid := (meterDBFloor + meterDBCeil) / 2
	if got := dbToFrac(mid); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("dbToFrac(mid) = %v, want ~0.5", got)
	}
}

func TestTruncateName(t *testing.T) {
	// Under the test stub, TextWidth returns debugCharW * runeCount. So a
	// 5-char name has width 5*debugCharW; with maxPx=3*debugCharW, scale=1
	// the function should chop to "abc".
	w3 := 3 * debugCharW
	if got := truncateName("abcdef", w3, 1.0); got != "abc" {
		t.Errorf("truncateName fit-3 = %q, want abc", got)
	}
	// Already-fits path.
	w10 := 10 * debugCharW
	if got := truncateName("abc", w10, 1.0); got != "abc" {
		t.Errorf("truncateName under-budget = %q, want abc", got)
	}
	// Pathological: maxPx 0 → falls through to name[:1].
	if got := truncateName("abcdef", 0, 1.0); got != "a" {
		t.Errorf("truncateName 0px = %q, want a", got)
	}
}
