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

func TestEnsureStepsBranches(t *testing.T) {
	t.Run("nonpositive_returns_zero_length_keeping_capacity", func(t *testing.T) {
		buf := make([]bool, 0, 8)
		out := ensureSteps(buf, 0)
		if len(out) != 0 {
			t.Fatalf("len=%d want 0", len(out))
		}
		if cap(out) != 8 {
			t.Fatalf("cap=%d want 8 (preserved)", cap(out))
		}
		out = ensureSteps(buf, -3)
		if len(out) != 0 || cap(out) != 8 {
			t.Fatalf("negative n: len=%d cap=%d want 0/8", len(out), cap(out))
		}
	})

	t.Run("growth_allocates_new_backing_array", func(t *testing.T) {
		buf := make([]bool, 2)
		buf[0] = true
		out := ensureSteps(buf, 5)
		if len(out) != 5 {
			t.Fatalf("len=%d want 5", len(out))
		}
		if cap(out) < 5 {
			t.Fatalf("cap=%d want >=5", cap(out))
		}
		// Verify a fresh allocation: out should be all-zero, original mutation invisible.
		for i, v := range out {
			if v {
				t.Fatalf("expected fresh zero slice, got out[%d]=true", i)
			}
		}
	})

	t.Run("reuse_reslices_underlying_array", func(t *testing.T) {
		buf := make([]bool, 4, 16)
		buf[0] = true
		out := ensureSteps(buf, 3)
		if len(out) != 3 || cap(out) != 16 {
			t.Fatalf("reuse: len=%d cap=%d want 3/16", len(out), cap(out))
		}
		// Mutating buf[0] must be visible through out.
		buf[0] = false
		if out[0] {
			t.Fatalf("expected out to share buf's backing array")
		}
	})
}

func TestEnsureTypesBranches(t *testing.T) {
	t.Run("nonpositive_returns_zero_length_keeping_capacity", func(t *testing.T) {
		buf := make([]model.NodeType, 0, 4)
		out := ensureTypes(buf, 0)
		if len(out) != 0 || cap(out) != 4 {
			t.Fatalf("len=%d cap=%d want 0/4", len(out), cap(out))
		}
	})
	t.Run("growth_allocates_new_backing_array", func(t *testing.T) {
		buf := make([]model.NodeType, 1)
		out := ensureTypes(buf, 4)
		if len(out) != 4 {
			t.Fatalf("len=%d want 4", len(out))
		}
		if cap(out) < 4 {
			t.Fatalf("cap=%d want >=4", cap(out))
		}
	})
	t.Run("reuse_reslices_underlying_array", func(t *testing.T) {
		buf := make([]model.NodeType, 8, 16)
		out := ensureTypes(buf, 5)
		if len(out) != 5 || cap(out) != 16 {
			t.Fatalf("reuse: len=%d cap=%d want 5/16", len(out), cap(out))
		}
	})
}

func TestBuildRowWindowEarlyReturns(t *testing.T) {
	t.Run("zero_length_returns_zero_slices", func(t *testing.T) {
		steps, types := BuildRowWindow(Config{Length: 0}, nil, nil, nil, nil, nil)
		if len(steps) != 0 || len(types) != 0 {
			t.Fatalf("len(steps)=%d len(types)=%d want 0/0", len(steps), len(types))
		}
	})
	t.Run("nil_callbacks_return_sized_zero_slices", func(t *testing.T) {
		steps, types := BuildRowWindow(Config{Length: 5}, nil, nil, nil, nil, nil)
		if len(steps) != 5 || len(types) != 5 {
			t.Fatalf("nil-cb: len(steps)=%d len(types)=%d want 5/5", len(steps), len(types))
		}
		for i, s := range steps {
			if s {
				t.Fatalf("expected zero steps; got steps[%d]=true", i)
			}
		}
	})
}
