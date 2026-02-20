package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func TestResolveMaskedStep_ImmutablePlayback(t *testing.T) {
	assertDefaultParityState(t)
	commit := commitView{val: false, typ: model.NodeTypeRegular, kind: timeline.CommitKindPlayback, ok: true}
	got := resolveMaskedStep(commit, true, true)
	if got {
		t.Fatalf("playback commit should remain masked; got %v", got)
	}
}

func TestResolveMaskedStep_InvisiblePlaybackStaysMasked(t *testing.T) {
	assertDefaultParityState(t)
	commit := commitView{val: false, typ: model.NodeTypeInvisible, kind: timeline.CommitKindPlayback, ok: true}
	got := resolveMaskedStep(commit, true, true)
	if got {
		t.Fatalf("invisible playback commit should remain masked; got %v", got)
	}
}

func TestResolveMaskedStep_SeededFollowsPredictor(t *testing.T) {
	assertDefaultParityState(t)
	commit := commitView{val: false, typ: model.NodeTypeRegular, kind: timeline.CommitKindSeeded, ok: true}
	got := resolveMaskedStep(commit, true, true)
	if !got {
		t.Fatalf("seeded commit should follow predictor; got %v", got)
	}
}
