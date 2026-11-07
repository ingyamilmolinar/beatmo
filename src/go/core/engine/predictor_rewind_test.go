package engine

import "testing"

// RebaseAt(abs) + Ensure must never leave the rewound region frozen empty: with the target below the window start
// (already evicted), RebaseAt resets windowStart and Ensure re-simulates forward from there.
func TestRebaseAtBackwardSeekResetsWindow(t *testing.T) {
	const windowCap = 4096
	pred := buildLoopedPredictor(t, windowCap)

	// Several cap-fulls, so windowStart really evicts rather than leaving an unfilled buffer.
	const forwardHorizon = 20_000
	pred.Ensure(forwardHorizon)

	start, end, _ := pred.WindowBoundsForTest()
	if end != forwardHorizon {
		t.Fatalf("Ensure(%d) did not extend horizon: windowEnd=%d", forwardHorizon, end)
	}
	if start == 0 {
		t.Fatalf("Ensure(%d) did not evict window forward: windowStart=%d, want >0", forwardHorizon, start)
	}

	// Backward seek: rebase to abs 8, which sits below the current
	// windowStart (i.e. within the already-evicted region).
	const seekTarget = 8
	if seekTarget >= start {
		t.Fatalf("test setup invalid: seekTarget=%d must be below windowStart=%d", seekTarget, start)
	}
	pred.RebaseAt(seekTarget)

	newStart := pred.WindowStart()
	if newStart > seekTarget {
		t.Fatalf("after RebaseAt(%d), WindowStart=%d must be <= %d", seekTarget, newStart, seekTarget)
	}

	// Re-simulate forward and confirm the loop's visible hits reappear in the
	// rewound region — proving the window was re-computed, not frozen empty.
	const reHorizon = 40
	pred.Ensure(reHorizon)

	sawHit := false
	for idx := seekTarget; idx < reHorizon; idx++ {
		if pred.VisibleAt(0, idx) {
			sawHit = true
			break
		}
	}
	if !sawHit {
		t.Fatalf("RebaseAt(%d)+Ensure(%d) produced no visible hits in [%d,%d) — window frozen empty (regression)",
			seekTarget, reHorizon, seekTarget, reHorizon)
	}
}
