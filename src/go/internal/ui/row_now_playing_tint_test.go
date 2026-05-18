//go:build test

package ui

import (
	"testing"
)

// TestMarkRowFiredSnapsToOneAndDecays verifies the now-playing row tint
// state machine on DrumView:
//   - MarkRowFired snaps the per-row decay value to 1.0;
//   - decayAnims attenuates it each frame so the tint fades out;
//   - out-of-range rows are silently ignored;
//   - the row slice grows as needed so newly added rows can be marked
//     without a separate initialization step.
func TestMarkRowFiredSnapsToOneAndDecays(t *testing.T) {
	assertDefaultParityState(t)

	dv := &DrumView{}

	// Initial: no decay state for any row.
	if got := dv.RowFireIntensity(0); got != 0 {
		t.Errorf("initial RowFireIntensity(0) = %v, want 0", got)
	}

	// Marking row 2 grows the slice and snaps to 1.0.
	dv.MarkRowFired(2)
	if got := dv.RowFireIntensity(2); got != 1.0 {
		t.Errorf("after MarkRowFired(2): intensity = %v, want 1.0", got)
	}
	if got := dv.RowFireIntensity(0); got != 0 {
		t.Errorf("MarkRowFired(2) leaked into row 0: intensity = %v, want 0", got)
	}

	// Decay: one frame should attenuate but stay positive; many frames drive to 0.
	dv.decayAnims()
	mid := dv.RowFireIntensity(2)
	if mid <= 0 || mid >= 1.0 {
		t.Errorf("after one decay tick: intensity = %v, want in (0, 1)", mid)
	}
	for i := 0; i < 200; i++ {
		dv.decayAnims()
	}
	if got := dv.RowFireIntensity(2); got != 0 {
		t.Errorf("after 200 decay ticks: intensity = %v, want 0", got)
	}

	// Out-of-range guard.
	dv.MarkRowFired(-1) // must not panic
	if got := dv.RowFireIntensity(-1); got != 0 {
		t.Errorf("RowFireIntensity(-1) = %v, want 0 (out of range)", got)
	}
}
