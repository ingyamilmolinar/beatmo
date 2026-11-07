package preview

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// Everything left of the VISUAL playhead renders from immutable history, even when a row's highlight index lags
// it (a delayed or dropped highlight right after an edit); otherwise those cells follow the edited predictor.
func TestBuildRowWindow_PastAnchoredToPlayheadNotLaggedHighlight(t *testing.T) {
	const (
		length       = 8
		nextIdx      = 2 // per-row highlight lagged behind the playhead
		elapsedBeats = 6 // visual playhead has passed abs 0..6
	)
	cfg := Config{
		Offset:            0,
		Length:            length,
		NextIdx:           nextIdx,
		ElapsedBeats:      elapsedBeats,
		FreezeLimit:       -1,
		PreservePastSteps: true, // playing
	}
	beatInfoAt := func(abs int) model.BeatInfo {
		_ = abs
		return model.BeatInfo{NodeType: model.NodeTypeRegular}
	}
	// Predictor AFTER a circuit edit: reports OFF everywhere.
	wantAt := func(abs int, bi model.BeatInfo) bool {
		_, _ = abs, bi
		return false
	}
	// ON for every abs the playhead reached.
	commitAt := func(abs int) (bool, model.NodeType, timeline.CommitKind, bool) {
		if abs < 0 || abs > elapsedBeats {
			return false, model.NodeTypeInvisible, timeline.CommitKindPlayback, false
		}
		return true, model.NodeTypeRegular, timeline.CommitKindPlayback, true
	}

	steps, _ := BuildRowWindow(cfg, beatInfoAt, wantAt, commitAt, nil, nil)

	// Committed history (true), not the post-edit predictor (false).
	for abs := 0; abs <= elapsedBeats; abs++ {
		if !steps[abs] {
			t.Fatalf("PAST MUTATED: abs=%d (< playhead=%d, highlight next=%d) rendered predictor=false instead of committed history=true; steps=%v",
				abs, elapsedBeats, nextIdx, steps)
		}
	}
}
