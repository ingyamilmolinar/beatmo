package preview

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// A past cell that has a played snapshot renders the snapshot and nothing
// else: not the live predictor, not the live path's node type, not a mutable
// commit, not the previous window. Future cells still follow the predictor.
func TestBuildRowWindow_PastRendersOnlyFromPlayedSnapshot(t *testing.T) {
	cfg := Config{
		Offset:            0,
		Length:            8,
		NextIdx:           5,
		ElapsedBeats:      4,
		FreezeLimit:       4,
		PreservePastSteps: true,
		PrevStepsOffset:   1, // window moved: prev-window preservation is off
		PrevSteps:         []bool{false, false, false, false, false, false, false, false},
	}
	// The live path now says every cell is an Invisible gap with no hit.
	beatInfoAt := func(abs int) model.BeatInfo { return model.BeatInfo{NodeType: model.NodeTypeInvisible} }
	wantAt := func(abs int, bi model.BeatInfo) bool { return false }
	// Commits are mutable kinds (what the lossy paths leave behind).
	commitAt := func(abs int) (bool, model.NodeType, timeline.CommitKind, bool) {
		return false, model.NodeTypeInvisible, timeline.CommitKindSeeded, true
	}
	// What actually played: a Regular hit on every even cell.
	playedAt := func(abs int) (bool, model.NodeType, bool) {
		if abs > 4 {
			return false, 0, false
		}
		return abs%2 == 0, model.NodeTypeRegular, true
	}
	cfg.PlayedAt = playedAt
	steps, types := BuildRowWindow(cfg, beatInfoAt, wantAt, commitAt, nil, nil)
	for abs := 0; abs <= 4; abs++ {
		if steps[abs] != (abs%2 == 0) || types[abs] != model.NodeTypeRegular {
			t.Fatalf("past abs=%d rendered (%v,%v); want the played snapshot (%v,Regular)", abs, steps[abs], types[abs], abs%2 == 0)
		}
	}
	for abs := 5; abs < 8; abs++ {
		if steps[abs] || types[abs] != model.NodeTypeInvisible {
			t.Fatalf("future abs=%d rendered (%v,%v); want the live predictor (false,Invisible)", abs, steps[abs], types[abs])
		}
	}
}

// Even an immutable Playback commit does not override what was shown: the
// snapshot is what the user saw as the cell played.
func TestBuildRowWindow_PlayedSnapshotOutranksCommits(t *testing.T) {
	cfg := Config{Offset: 0, Length: 2, NextIdx: 2, ElapsedBeats: 1, FreezeLimit: -1}
	beatInfoAt := func(abs int) model.BeatInfo { return model.BeatInfo{NodeType: model.NodeTypeRegular} }
	wantAt := func(abs int, bi model.BeatInfo) bool { return true }
	commitAt := func(abs int) (bool, model.NodeType, timeline.CommitKind, bool) {
		return true, model.NodeTypeRegular, timeline.CommitKindPlayback, true
	}
	cfg.PlayedAt = func(abs int) (bool, model.NodeType, bool) { return false, model.NodeTypeSilent, true }
	steps, types := BuildRowWindow(cfg, beatInfoAt, wantAt, commitAt, nil, nil)
	if steps[0] || types[0] != model.NodeTypeSilent {
		t.Fatalf("abs=0 rendered (%v,%v); want played (false,Silent)", steps[0], types[0])
	}
}
