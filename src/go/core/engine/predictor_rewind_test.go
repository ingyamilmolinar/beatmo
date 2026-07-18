package engine

import "testing"

// TestRebaseAtBackwardSeekResetsWindow validates the composition that the
// timeline-seek chokepoint (Task 6) relies on: RebaseAt(abs) followed by
// Ensure(...) must never leave the rewound region "frozen empty". When the
// seek target sits below the current sliding-window start (the window has
// already evicted that history), RebaseAt must reset windowStart to the
// target and a subsequent Ensure must re-simulate forward from there,
// re-populating visible/audible hits rather than leaving stale/empty
// buffers.
//
// Reuses buildLoopedPredictor (predictor_window_bounded_test.go) — a 6-row
// predictor whose row 0 loops a 2-node (n0<->n1) cycle — rather than
// inventing a new graph builder.
func TestRebaseAtBackwardSeekResetsWindow(t *testing.T) {
	const windowCap = 4096
	pred := buildLoopedPredictor(t, windowCap)

	// Drive the window forward past several cap-fulls so windowStart advances
	// well above 0 (forcing real eviction, not just an unfilled buffer).
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
