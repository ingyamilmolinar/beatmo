//go:build test

package ui

import "testing"

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
	// Internal beat 60 at 0.5 s/beat is 30 s, shown as the 1-based "Beat 61".
	p := playheadPosAt(60, 0.5, 8)
	if got := formatBeatTime(p); got != "Beat 61 · 0:30" {
		t.Fatalf("formatBeatTime=%q want %q", got, "Beat 61 · 0:30")
	}
}
