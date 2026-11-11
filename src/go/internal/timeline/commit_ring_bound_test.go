package timeline

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestCommitRingBounded asserts that commitRing capacity and the immutables
// sidecar both stay bounded under sustained playback-kind appends. The
// production WASM OOM at ~55 min of playback was driven in part by these two
// timeline structures growing unbounded (the other half being
// game.paritySeqDecisions; see TestParityDecisionsBoundedUnderScanStarvation).
//
// Bounds reflect the retention policy in service.go:
//   - commitRing cap ≤ commitRingCapMultiplier × loopLen (= 8 × 64 = 512).
//     setCapMax in append() drops the oldest entry once the ceiling is hit;
//     immutables preserves the trimmed history for any consumer that needs it.
//   - immutables[row] ≤ immutablesPerRowMax (= 1024). pruneImmutablesRow
//     drops abs below (latestAbs - cap + 1) at each write.
func TestCommitRingBounded(t *testing.T) {
	const loopLen = 64
	const appends = 100_000
	const expectedCapBound = commitRingCapMultiplier * loopLen
	const expectedImmutBound = immutablesPerRowMax

	svc := NewService()
	for abs := 0; abs < appends; abs++ {
		svc.RecordCommit(0 /*row*/, abs, true, model.NodeTypeRegular, loopLen, loopLen)
	}

	size, capa, immut := svc.RingLenForTest(0)
	if capa > expectedCapBound {
		t.Errorf("commitRing cap=%d after %d appends; want ≤ %d", capa, appends, expectedCapBound)
	}
	if immut > expectedImmutBound {
		t.Errorf("Service.immutables[row]=%d after %d appends; want ≤ %d", immut, appends, expectedImmutBound)
	}
	if size > capa {
		t.Errorf("commitRing size=%d > cap=%d", size, capa)
	}
}
