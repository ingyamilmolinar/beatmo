package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

// commitView carries a snapshot of a timeline commit for masking decisions.
// kind differentiates immutable playback/import history from speculative seeds.
type commitView struct {
	val  bool
	typ  model.NodeType
	kind timeline.CommitKind
	ok   bool
}

// resolveMaskedStep decides the final step for a frozen cell. Immutable
// playback/import commits win only for the true past (abs < nextBeatIdxs[row]).
// All other commit kinds are treated as soft masks and follow the predictor.
//
// This function is intentionally pure: it must not mutate timeline state. Any
// reconciliation (ReplaceCommit/Trim/etc.) belongs in the refresh reconciliation
// phase so window building stays deterministic and easy to debug.
func resolveMaskedStep(commit commitView, want bool, inPast bool) bool {
	if !commit.ok {
		return want
	}
	switch commit.kind {
	case timeline.CommitKindPlayback, timeline.CommitKindImport:
		if !inPast {
			return want
		}
		return commit.val
	default:
		return want
	}
}
