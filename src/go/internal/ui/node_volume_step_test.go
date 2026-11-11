package ui

import (
	"math"
	"testing"
)

// TestStepNodeVolumeDb verifies each +/- click is a fixed 1 dB step, with no
// upper cap and a mute floor that lets the value reach exactly 0.
func TestStepNodeVolumeDb(t *testing.T) {
	approx := func(got, want float64) bool { return math.Abs(got-want) < 1e-6 }

	// One down-click from unity is exactly −nodeVolStepDb (1 dB).
	down := stepNodeVolume(1.0, false)
	if wantDb := 20 * math.Log10(down); math.Abs(wantDb+nodeVolStepDb) > 1e-9 {
		t.Errorf("down step from 1.0: got %.4f (%.3f dB), want %.3f dB", down, wantDb, -nodeVolStepDb)
	}

	// Up then down round-trips back to unity.
	if got := stepNodeVolume(stepNodeVolume(1.0, true), false); !approx(got, 1.0) {
		t.Errorf("up then down: got %.6f, want 1.0", got)
	}

	// Repeated down-clicks eventually snap to 0 (mute), never negative.
	v := 1.0
	for i := 0; i < 200; i++ {
		v = stepNodeVolume(v, false)
		if v < 0 {
			t.Fatalf("volume went negative: %.6f", v)
		}
	}
	if v != 0 {
		t.Errorf("after many down-clicks: got %.6f, want 0 (mute)", v)
	}

	// Stepping up from mute returns a positive level, and down again returns to mute.
	unmuted := stepNodeVolume(0, true)
	if unmuted <= 0 {
		t.Errorf("up from mute: got %.6f, want > 0", unmuted)
	}
	if got := stepNodeVolume(unmuted, false); got != 0 {
		t.Errorf("down from just-unmuted: got %.6f, want 0", got)
	}

	// Stepping down from mute stays muted.
	if got := stepNodeVolume(0, false); got != 0 {
		t.Errorf("down from mute: got %.6f, want 0", got)
	}

	// Repeated up-clicks are UNBOUNDED — no cap.
	v = 1.0
	for i := 0; i < 50; i++ {
		v = stepNodeVolume(v, true)
	}
	// 50 × +1 dB = +50 dB ≈ 316×; assert it climbed well past any old cap.
	if v < 100 {
		t.Errorf("up should be unbounded: after 50 clicks got %.4f, want >= 100", v)
	}
}

func TestFormatNodeVolumeDb(t *testing.T) {
	cases := []struct {
		v    float64
		want string
	}{
		{1.0, "0 dB"},
		{0.5, "-6 dB"},  // 20*log10(0.5) = -6.02
		{2.0, "+6 dB"},  // +6.02
		{0.75, "-2 dB"}, // -2.50 -> round -2 (banker's-free math.Round)
		{0, "Muted"},    // KeyAudMuted (EN)
	}
	for _, c := range cases {
		if got := formatNodeVolumeDb(c.v); got != c.want {
			t.Errorf("formatNodeVolumeDb(%.3f): got %q, want %q", c.v, got, c.want)
		}
	}
}
