//go:build test

package ui

import "testing"

func TestParseClock(t *testing.T) {
	cases := []struct {
		s   string
		sec float64
		ok  bool
	}{
		{"0:30", 30, true}, {"1:05", 65, true}, {"12", 12, true}, {"2:00", 120, true},
		{"", 0, false}, {"a:b", 0, false}, {"1:99", 0, false},
	}
	for _, c := range cases {
		sec, ok := parseClock(c.s)
		if ok != c.ok || (ok && sec != c.sec) {
			t.Fatalf("parseClock(%q)=(%v,%v) want (%v,%v)", c.s, sec, ok, c.sec, c.ok)
		}
	}
}

func TestTimeToStepsClosestBeatAtOrAfter(t *testing.T) {
	div, bpm := 8, 120 // 2 beats/sec → 30s = beat 60
	maxSteps := 1000 * div
	// 30s → exactly beat 60 → 60*div.
	if got := timeToSteps(30, bpm, div, maxSteps); got != 60*div {
		t.Fatalf("timeToSteps(30)=%d want %d", got, 60*div)
	}
	// 30.1s → 60.2 beats → ceil to beat 61.
	if got := timeToSteps(30.1, bpm, div, maxSteps); got != 61*div {
		t.Fatalf("timeToSteps(30.1)=%d want %d (ceil to next beat)", got, 61*div)
	}
	// clamp above song length.
	if got := timeToSteps(1e9, bpm, div, maxSteps); got != maxSteps {
		t.Fatalf("timeToSteps(huge)=%d want %d (clamped)", got, maxSteps)
	}
	// empty timeline (maxSteps==0) clamps to 0, per [0, maxSteps].
	if got := timeToSteps(30, bpm, div, 0); got != 0 {
		t.Fatalf("timeToSteps(30, maxSteps=0)=%d want 0 (empty timeline)", got)
	}
}

func TestFormatClock(t *testing.T) {
	cases := []struct {
		sec  int
		want string
	}{
		{0, "0:00"}, {5, "0:05"}, {65, "1:05"}, {600, "10:00"}, {-3, "0:00"},
	}
	for _, c := range cases {
		if got := formatClock(c.sec); got != c.want {
			t.Fatalf("formatClock(%d)=%q want %q", c.sec, got, c.want)
		}
	}
}

func TestFormatBeatTime(t *testing.T) {
	// beat 60 at 120bpm, div 8 → "Beat 60 · 0:30".
	if got := formatBeatTime(60*8, 8, 120); got != "Beat 60 · 0:30" {
		t.Fatalf("formatBeatTime=%q want %q", got, "Beat 60 · 0:30")
	}
}
