package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Only the NodeID matters; synthesized invisible steps use InvalidNodeID.
func bi(id model.NodeID) model.BeatInfo { return model.BeatInfo{NodeID: id} }

// Synthesized in-between steps are part of the path.
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

// The index of the NEXT occurrence of the origin: the unrepeated length the predictor consumes.
func TestRawBeatLenLoopReturnsSegmentLength(t *testing.T) {
	// 3-node loop: 1 → 2 → 3 → 1 → 2 → 3 (repeated to fill beatLen).
	path := []model.BeatInfo{bi(1), bi(2), bi(3), bi(1), bi(2), bi(3)}
	if got := rawBeatLen(path, true, 0); got != 3 {
		t.Errorf("rawBeatLen(3-node loop @0) = %d, want 3", got)
	}
}

func TestRawBeatLenLoopWithLeadIn(t *testing.T) {
	// 10 → 11 → 1 → 2 → 3 → 1 → 2 → 3
	path := []model.BeatInfo{bi(10), bi(11), bi(1), bi(2), bi(3), bi(1), bi(2), bi(3)}
	// loopStart=2 (where node 1 begins the loop), origin=1, next 1 at i=5.
	if got := rawBeatLen(path, true, 2); got != 5 {
		t.Errorf("rawBeatLen(lead-in + 3-node loop) = %d, want 5", got)
	}
}

// An origin pointing to itself: segment length 1.
func TestRawBeatLenSingleNodeLoop(t *testing.T) {
	path := []model.BeatInfo{bi(7), bi(7), bi(7), bi(7)}
	if got := rawBeatLen(path, true, 0); got != 1 {
		t.Errorf("rawBeatLen(single-node loop) = %d, want 1", got)
	}
}

// A bogus loopStart returns len(path), no panic.
func TestRawBeatLenLoopStartOutOfRange(t *testing.T) {
	path := []model.BeatInfo{bi(1), bi(2)}
	if got := rawBeatLen(path, true, -1); got != len(path) {
		t.Errorf("rawBeatLen(loopStart=-1) = %d, want %d", got, len(path))
	}
	if got := rawBeatLen(path, true, 99); got != len(path) {
		t.Errorf("rawBeatLen(loopStart=99) = %d, want %d", got, len(path))
	}
}

// Falls back to len(path) rather than scanning off the end.
func TestRawBeatLenLoopOriginNeverRepeats(t *testing.T) {
	path := []model.BeatInfo{bi(1), bi(2), bi(3)}
	if got := rawBeatLen(path, true, 0); got != len(path) {
		t.Errorf("rawBeatLen(origin-once loop) = %d, want %d", got, len(path))
	}
}

// rawBeatLen returns the next origin's index and loopSegmentLen the distance, equal when loopStart is 0.
func TestLoopSegmentLenMatchesRawBeatLen(t *testing.T) {
	path := []model.BeatInfo{bi(1), bi(2), bi(3), bi(1), bi(2), bi(3)}
	if got := loopSegmentLen(path, 0); got != 3 {
		t.Errorf("loopSegmentLen = %d, want 3", got)
	}
	if got := loopSegmentLen(path, 3); got != 3 {
		t.Errorf("loopSegmentLen(start=3) = %d, want 3", got)
	}
}

// A count, not an index.
func TestLoopSegmentLenOriginNeverRepeats(t *testing.T) {
	path := []model.BeatInfo{bi(1), bi(2), bi(3)}
	if got := loopSegmentLen(path, 0); got != 3 {
		t.Errorf("loopSegmentLen(origin-once) = %d, want 3", got)
	}
	if got := loopSegmentLen(path, 1); got != 2 {
		t.Errorf("loopSegmentLen(start=1, origin-once) = %d, want 2", got)
	}
}

// Returns 0 so callers write a zero instead of crashing.
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

// Row in the high 16 bits, beat index in the low 16.
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
