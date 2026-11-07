package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// The slate keeps a *Game back-reference by design: the coupling surface is too wide for an interface to decouple
// anything. Pure helpers need no Game; the rest run on a minimal Game literal with the back-ref set, as New() does.

func TestPlaybackSlate_DeterministicRoll(t *testing.T) {
	s := &playbackSlate{} // pure hash: no *Game needed

	// Deterministic: identical inputs must yield an identical roll.
	if a, b := s.deterministicRoll(2, 5, model.NodeID(7)), s.deterministicRoll(2, 5, model.NodeID(7)); a != b {
		t.Fatalf("deterministicRoll not deterministic: %v != %v", a, b)
	}

	// Output must land in [0,1) across a spread of inputs (incl. negatives).
	for _, tc := range []struct{ row, idx, id int }{{0, 0, 0}, {1, 2, 3}, {100, 7, 42}, {-1, -1, 9}} {
		if v := s.deterministicRoll(tc.row, tc.idx, model.NodeID(tc.id)); v < 0 || v >= 1 {
			t.Errorf("deterministicRoll(%d,%d,%d)=%v out of [0,1)", tc.row, tc.idx, tc.id, v)
		}
	}

	// Distinct inputs must not collapse to one value (guards a degenerate hash).
	if s.deterministicRoll(0, 0, 0) == s.deterministicRoll(1, 0, 0) {
		t.Error("deterministicRoll collapsed distinct inputs to the same value")
	}
}

func TestPlaybackSlate_WrapBeatIndexRow(t *testing.T) {
	g := &Game{}
	g.playbackSlate.g = g // bare literal bypasses New(); set the back-ref (see New()).

	// Row 0: 4 beats, non-looping. Row 1: 6 beats, looping from index 2 (loopLen=4).
	g.beatInfosByRow = [][]model.BeatInfo{
		make([]model.BeatInfo, 4),
		make([]model.BeatInfo, 6),
	}
	g.isLoopByRow = []bool{false, true}
	g.loopStartByRow = []int{0, 2}

	cases := []struct {
		name           string
		row, idx, want int
	}{
		{"row out of range -> 0", 5, 0, 0},
		{"negative idx -> 0", 0, -1, 0},
		{"idx in range -> idx", 0, 2, 2},
		{"non-loop overflow clamps to last", 0, 10, 3},
		{"loop wrap to start of loop", 1, 6, 2}, // 2 + (6-2)%4 = 2
		{"loop wrap into loop body", 1, 9, 5},   // 2 + (9-2)%4 = 5
	}
	for _, tc := range cases {
		if got := g.wrapBeatIndexRow(tc.row, tc.idx); got != tc.want {
			t.Errorf("%s: wrapBeatIndexRow(%d,%d)=%d, want %d", tc.name, tc.row, tc.idx, got, tc.want)
		}
	}
}
