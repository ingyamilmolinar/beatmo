package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

type commitView struct {
	val  bool
	typ  model.NodeType
	kind timeline.CommitKind
	ok   bool
}

// Immutable playback/import commits win only for the true past; every other kind is a soft mask that follows the
// predictor. Pure: reconciliation belongs in the refresh phase, so window building stays deterministic.
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
