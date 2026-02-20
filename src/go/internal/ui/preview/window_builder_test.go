package preview

import (
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func TestBuildRowWindow_ImmutablePlaybackOverridesPredictorInPast(t *testing.T) {
	cfg := Config{
		Offset:      0,
		Length:      8,
		NextIdx:     4,
		FreezeLimit: -1,
	}
	beatInfoAt := func(abs int) model.BeatInfo {
		_ = abs
		return model.BeatInfo{NodeType: model.NodeTypeRegular}
	}
	wantAt := func(abs int, bi model.BeatInfo) bool {
		_ = abs
		_ = bi
		return true
	}
	commitAt := func(abs int) (bool, model.NodeType, timeline.CommitKind, bool) {
		if abs < 0 || abs >= 4 {
			return false, model.NodeTypeInvisible, timeline.CommitKindPlayback, false
		}
		val := abs%2 == 0
		typ := model.NodeTypeRegular
		if abs == 2 {
			typ = model.NodeTypeMute
		}
		return val, typ, timeline.CommitKindPlayback, true
	}

	steps, types := BuildRowWindow(cfg, beatInfoAt, wantAt, commitAt, nil, nil)
	if got, want := steps, []bool{true, false, true, false, true, true, true, true}; !slices.Equal(got, want) {
		t.Fatalf("steps mismatch:\n got=%v\nwant=%v", got, want)
	}
	if types[2] != model.NodeTypeMute {
		t.Fatalf("expected committed type at abs=2 to win; got %v", types[2])
	}
}

func TestBuildRowWindow_SeededCommitFollowsPredictorInPast(t *testing.T) {
	cfg := Config{
		Offset:      0,
		Length:      4,
		NextIdx:     4,
		FreezeLimit: -1,
	}
	beatInfoAt := func(abs int) model.BeatInfo {
		_ = abs
		return model.BeatInfo{NodeType: model.NodeTypeRegular}
	}
	wantAt := func(abs int, bi model.BeatInfo) bool {
		_ = abs
		_ = bi
		return true
	}
	commitAt := func(abs int) (bool, model.NodeType, timeline.CommitKind, bool) {
		if abs != 1 {
			return false, model.NodeTypeInvisible, timeline.CommitKindPlayback, false
		}
		return false, model.NodeTypeRegular, timeline.CommitKindSeeded, true
	}

	steps, _ := BuildRowWindow(cfg, beatInfoAt, wantAt, commitAt, nil, nil)
	if !steps[1] {
		t.Fatalf("expected seeded commit in past to follow predictor=true; got false (steps=%v)", steps)
	}
}

func TestBuildRowWindow_PreservesPastFromPrevWindowWhenNoImmutableCommit(t *testing.T) {
	prevSteps := []bool{false, true, false, true, true, true, true, true}
	prevTypes := []model.NodeType{
		model.NodeTypeRegular, model.NodeTypeMute, model.NodeTypeRegular, model.NodeTypeRegular,
		model.NodeTypeRegular, model.NodeTypeRegular, model.NodeTypeRegular, model.NodeTypeRegular,
	}
	cfg := Config{
		Offset:            0,
		Length:            8,
		NextIdx:           4,
		ElapsedBeats:      8,
		FreezeLimit:       -1,
		PreservePastSteps: true,
		PrevStepsOffset:   0,
		PrevSteps:         prevSteps,
		PrevTypesOffset:   0,
		PrevTypes:         prevTypes,
		FastPath:          false,
	}
	beatInfoAt := func(abs int) model.BeatInfo {
		_ = abs
		return model.BeatInfo{NodeType: model.NodeTypeRegular}
	}
	wantAt := func(abs int, bi model.BeatInfo) bool {
		_ = abs
		_ = bi
		return false
	}
	commitAt := func(abs int) (bool, model.NodeType, timeline.CommitKind, bool) {
		_ = abs
		return false, model.NodeTypeInvisible, timeline.CommitKindPlayback, false
	}

	steps, types := BuildRowWindow(cfg, beatInfoAt, wantAt, commitAt, nil, nil)
	if got, want := steps[:4], prevSteps[:4]; !slices.Equal(got, want) {
		t.Fatalf("expected past steps preserved:\n got=%v\nwant=%v", got, want)
	}
	for i := 4; i < len(steps); i++ {
		if steps[i] {
			t.Fatalf("expected future steps to follow predictor=false; got steps=%v", steps)
		}
	}
	if got, want := types[:4], prevTypes[:4]; !slices.Equal(got, want) {
		t.Fatalf("expected past types preserved:\n got=%v\nwant=%v", got, want)
	}
	for i := 4; i < len(types); i++ {
		if types[i] != model.NodeTypeRegular {
			t.Fatalf("expected future types to follow beatInfo regular; got %v (types=%v)", types[i], types)
		}
	}
}
