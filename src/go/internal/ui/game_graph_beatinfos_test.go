package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// bi is a tiny constructor for BeatInfo in tests — we only care about the
// NodeID dimension. Synthesized "invisible" steps use InvalidNodeID.
func bi(id model.NodeID) model.BeatInfo { return model.BeatInfo{NodeID: id} }

// TestRawBeatLenNonLoopReturnsFullPath pins the documented branch: when
// the path is not a loop, raw length equals the path length verbatim,
// regardless of loopStart. (Synthesized in-between steps are part of the
// path.)
func TestRawBeatLenNonLoopReturnsFullPath(t *testing.T) {
	path := []model.BeatInfo{bi(1), bi(2), bi(3), bi(4)}
	if got := rawBeatLen(path, false, 0); got != len(path) {
		t.Errorf("rawBeatLen(non-loop) = %d, want %d", got, len(path))
	}
	// loopStart is ignored when isLoop=false.
	if got := rawBeatLen(path, false, 99); got != len(path) {
		t.Errorf("rawBeatLen(non-loop, bad start) = %d, want %d", got, len(path))
	}
}

// TestRawBeatLenLoopReturnsSegmentLength scans forward from loopStart and
// returns the index of the *next* occurrence of the loop origin. This is
// the unrepeated traversal length the predictor consumes.
func TestRawBeatLenLoopReturnsSegmentLength(t *testing.T) {
	// 3-node loop: 1 → 2 → 3 → 1 → 2 → 3 (repeated to fill beatLen).
	path := []model.BeatInfo{bi(1), bi(2), bi(3), bi(1), bi(2), bi(3)}
	if got := rawBeatLen(path, true, 0); got != 3 {
		t.Errorf("rawBeatLen(3-node loop @0) = %d, want 3", got)
	}
}

// TestRawBeatLenLoopWithLeadIn covers the prefix-before-loop case: the
// path enters the loop after some lead-in nodes, so loopStart > 0.
func TestRawBeatLenLoopWithLeadIn(t *testing.T) {
	// 10 → 11 → 1 → 2 → 3 → 1 → 2 → 3
	path := []model.BeatInfo{bi(10), bi(11), bi(1), bi(2), bi(3), bi(1), bi(2), bi(3)}
	// loopStart=2 (where node 1 begins the loop), origin=1, next 1 at i=5.
	if got := rawBeatLen(path, true, 2); got != 5 {
		t.Errorf("rawBeatLen(lead-in + 3-node loop) = %d, want 5", got)
	}
}

// TestRawBeatLenSingleNodeLoop covers the degenerate single-node loop: an
// origin pointing to itself. The unbounded path is [origin, origin, …];
// segment length is 1.
func TestRawBeatLenSingleNodeLoop(t *testing.T) {
	path := []model.BeatInfo{bi(7), bi(7), bi(7), bi(7)}
	if got := rawBeatLen(path, true, 0); got != 1 {
		t.Errorf("rawBeatLen(single-node loop) = %d, want 1", got)
	}
}

// TestRawBeatLenLoopStartOutOfRange is the safety-fallback branch: a
// nonsensical loopStart must not panic and must return len(path) so the
// caller is no worse off than with an empty/bogus loop annotation.
func TestRawBeatLenLoopStartOutOfRange(t *testing.T) {
	path := []model.BeatInfo{bi(1), bi(2)}
	if got := rawBeatLen(path, true, -1); got != len(path) {
		t.Errorf("rawBeatLen(loopStart=-1) = %d, want %d", got, len(path))
	}
	if got := rawBeatLen(path, true, 99); got != len(path) {
		t.Errorf("rawBeatLen(loopStart=99) = %d, want %d", got, len(path))
	}
}

// TestRawBeatLenLoopOriginNeverRepeats covers the "origin only appears
// once" pathological case — the helper must fall back to len(path) rather
// than scanning off the end.
func TestRawBeatLenLoopOriginNeverRepeats(t *testing.T) {
	path := []model.BeatInfo{bi(1), bi(2), bi(3)}
	if got := rawBeatLen(path, true, 0); got != len(path) {
		t.Errorf("rawBeatLen(origin-once loop) = %d, want %d", got, len(path))
	}
}

// TestLoopSegmentLenMatchesRawBeatLen — the two helpers compute the same
// quantity for the loop case (rawBeatLen pegs the index of the next
// origin; loopSegmentLen reports the *distance*, which is the same since
// rawBeatLen returns the index when loopStart=0).
func TestLoopSegmentLenMatchesRawBeatLen(t *testing.T) {
	path := []model.BeatInfo{bi(1), bi(2), bi(3), bi(1), bi(2), bi(3)}
	if got := loopSegmentLen(path, 0); got != 3 {
		t.Errorf("loopSegmentLen = %d, want 3", got)
	}
	if got := loopSegmentLen(path, 3); got != 3 {
		t.Errorf("loopSegmentLen(start=3) = %d, want 3", got)
	}
}

// TestLoopSegmentLenOriginNeverRepeats falls back to "length from start
// to end" when the origin doesn't recur — same shape as rawBeatLen but
// the return value is the *count*, not an index.
func TestLoopSegmentLenOriginNeverRepeats(t *testing.T) {
	path := []model.BeatInfo{bi(1), bi(2), bi(3)}
	if got := loopSegmentLen(path, 0); got != 3 {
		t.Errorf("loopSegmentLen(origin-once) = %d, want 3", got)
	}
	if got := loopSegmentLen(path, 1); got != 2 {
		t.Errorf("loopSegmentLen(start=1, origin-once) = %d, want 2", got)
	}
}

// TestLoopSegmentLenOutOfRange covers the safety fallback: a bogus start
// must not panic — returns 0 so callers (e.g. updateBeatInfos) write a
// zero into loopLenByRow instead of crashing.
func TestLoopSegmentLenOutOfRange(t *testing.T) {
	path := []model.BeatInfo{bi(1), bi(2)}
	if got := loopSegmentLen(path, -1); got != 0 {
		t.Errorf("loopSegmentLen(-1) = %d, want 0", got)
	}
	if got := loopSegmentLen(path, 99); got != 0 {
		t.Errorf("loopSegmentLen(99) = %d, want 0", got)
	}
	if got := loopSegmentLen(nil, 0); got != 0 {
		t.Errorf("loopSegmentLen(nil) = %d, want 0", got)
	}
}

// TestBeatKeyPackRoundTrip exercises the makeBeatKey / splitBeatKey
// inverse used across the highlight + parity layers. The high 16 bits
// carry the row index; the low 16 carry the per-row beat index.
func TestBeatKeyPackRoundTrip(t *testing.T) {
	cases := []struct{ row, idx int }{
		{0, 0},
		{1, 1},
		{12, 345},
		{255, 65535},
		{0, 65535},
		{32767, 0},
	}
	for _, c := range cases {
		key := makeBeatKey(c.row, c.idx)
		row, idx := splitBeatKey(key)
		if row != c.row || idx != c.idx {
			t.Errorf("roundtrip (row=%d idx=%d) -> key=%d -> (row=%d idx=%d)", c.row, c.idx, key, row, idx)
		}
	}
}
