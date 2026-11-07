package timeline

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// The ring and the immutables sidecar stay bounded under sustained playback appends (unbounded, they fed a WASM OOM in
// long sessions): ring ≤ commitRingCapMultiplier × loopLen, immutables[row] ≤ immutablesPerRowMax, the trimmed history
// kept elsewhere.
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
