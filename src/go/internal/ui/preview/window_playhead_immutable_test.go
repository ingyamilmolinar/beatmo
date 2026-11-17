package preview

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// Regression: everything strictly left of the VISUAL playhead (ElapsedBeats)
// is past and must render from the immutable timeline/history commits — never
// from the live predictor. This must hold even when the per-row highlight
// index (NextIdx) lags the playhead, which happens in production whenever a
// non-primary row's highlight is delayed or dropped (the stale-info guard in
// applySequencerHighlight fires right after a circuit edit). Before the fix,
// the past boundary was NextIdx alone, so cells in [NextIdx, ElapsedBeats]
// followed the predictor and mutated when an edit changed it.
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
	// Immutable playback history recorded as the playhead passed: ON for every
	// abs the playhead has reached (0..elapsedBeats).
	commitAt := func(abs int) (bool, model.NodeType, timeline.CommitKind, bool) {
		if abs < 0 || abs > elapsedBeats {
			return false, model.NodeTypeInvisible, timeline.CommitKindPlayback, false
		}
		return true, model.NodeTypeRegular, timeline.CommitKindPlayback, true
	}

	steps, _ := BuildRowWindow(cfg, beatInfoAt, wantAt, commitAt, nil, nil)

	// Every cell strictly left of the playhead must reflect committed history
	// (true), not the post-edit predictor (false).
	for abs := 0; abs <= elapsedBeats; abs++ {
		if !steps[abs] {
			t.Fatalf("PAST MUTATED: abs=%d (< playhead=%d, highlight next=%d) rendered predictor=false instead of committed history=true; steps=%v",
				abs, elapsedBeats, nextIdx, steps)
		}
	}
}
