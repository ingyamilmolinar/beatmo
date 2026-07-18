//go:build test

package ui

import "testing"

// TestPositionSpecParsesBeatAndSubdiv pins the load-bearing parser behind the
// tappable beat counter: text grammar is "beat" or "beat.subdiv", parsed on
// the raw string (not a float) so "17.10" is unambiguous.
func TestPositionSpecParsesBeatAndSubdiv(t *testing.T) {
	div := 8
	maxSteps := 64 * div
	spec := positionValueSpec(div, maxSteps)

	cases := []struct {
		text string
		want float64 // steps
		ok   bool
	}{
		{"17", float64(17 * div), true},     // whole beat
		{"17.3", float64(17*div + 3), true}, // beat 17, subdivision index 3
		{"0.0", 0, true},                    // origin
		{"2.7", float64(2*div + 7), true},   // last subdiv at div=8
		{"2.9", float64(2*div + 7), true},   // subdiv clamped to div-1
		{"", 0, false},                      // empty rejected
		{"abc", 0, false},                   // garbage rejected
	}
	for _, c := range cases {
		got, ok := spec.Parse(c.text)
		if ok != c.ok {
			t.Fatalf("Parse(%q) ok=%v want %v", c.text, ok, c.ok)
		}
		if ok && got != c.want {
			t.Fatalf("Parse(%q)=%v want %v", c.text, got, c.want)
		}
	}
}

// TestPositionSpecStringParseNotFloat pins THE load-bearing property that a
// float-based editor cannot provide: the fractional part is a subdivision
// INDEX parsed from the raw string, not a decimal. At div=20 the subdivisions
// 10 and 1 are both in range (unclamped), so ".10" and ".1" must resolve to
// DIFFERENT steps — whereas strconv.ParseFloat("5.10") == ParseFloat("5.1"),
// which would collapse them. div=8 (the other cases) can't show this because
// subdiv 10 clamps to 7 and masks the divergence.
func TestPositionSpecStringParseNotFloat(t *testing.T) {
	div := 20
	spec := positionValueSpec(div, 64*div)
	got10, ok10 := spec.Parse("5.10")
	got1, ok1 := spec.Parse("5.1")
	if !ok10 || !ok1 {
		t.Fatalf("Parse ok10=%v ok1=%v, want both true", ok10, ok1)
	}
	if want := float64(5*div + 10); got10 != want {
		t.Fatalf(`Parse("5.10")=%v want %v (beat 5, subdiv 10)`, got10, want)
	}
	if want := float64(5*div + 1); got1 != want {
		t.Fatalf(`Parse("5.1")=%v want %v (beat 5, subdiv 1)`, got1, want)
	}
	if got10 == got1 {
		t.Fatalf("Parse(\"5.10\")==Parse(\"5.1\")==%v — parser collapsed them like a float would", got10)
	}
	// Round-trip: Format of the subdiv-10 step must render ".10", not ".1".
	if s := spec.Format(got10); s != "5.10" {
		t.Fatalf("Format(%v)=%q want 5.10", got10, s)
	}
}

// TestPositionSpecFormatsBeatDotSubdiv pins the inverse: Format never emits a
// trailing ".0" on a beat boundary.
func TestPositionSpecFormatsBeatDotSubdiv(t *testing.T) {
	div := 8
	spec := positionValueSpec(div, 64*div)
	if s := spec.Format(float64(17*div + 3)); s != "17.3" {
		t.Fatalf("Format(139)=%q want 17.3", s)
	}
	if s := spec.Format(float64(17 * div)); s != "17" {
		t.Fatalf("Format(136)=%q want 17 (no .0 suffix on beat boundary)", s)
	}
}
