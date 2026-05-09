//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// TestCheckComponentBounds_TripsParityAudio verifies that the hard
// per-component bound on g.parityAudio fires when the drop-oldest cap in
// recordParityAudio is bypassed. This is the assertion that long-running
// memory soaks rely on to catch a future regression where someone removes
// or bypasses the cap.
func TestCheckComponentBounds_TripsParityAudio(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// Inflate parityAudio past the bound (1024 entries). The slice elements'
	// content does not matter — checkComponentBounds asserts only on length.
	g.parityAudio = make([]parityAudioEvent, 1025)

	fails := checkComponentBounds(g, soakScenario{name: "synthetic"}, 8400)
	if !anyContains(fails, "parityAudio") {
		t.Fatalf("expected parityAudio bound to trip; got fails=%v", fails)
	}
}

// TestCheckComponentBounds_TripsParitySeqDecisions verifies the per-row map
// bound trips when parityPrune fails to keep up with playback rate.
func TestCheckComponentBounds_TripsParitySeqDecisions(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	if g.paritySeqDecisions == nil {
		g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
	}
	row0 := make(map[int]paritySeqDecision, 4097)
	for i := 0; i < 4097; i++ {
		row0[i] = paritySeqDecision{Row: 0, Abs: i}
	}
	g.paritySeqDecisions[0] = row0

	fails := checkComponentBounds(g, soakScenario{name: "synthetic"}, 8400)
	if !anyContains(fails, "paritySeqDecisions[row=0]") {
		t.Fatalf("expected paritySeqDecisions bound to trip; got fails=%v", fails)
	}
}

// TestCheckComponentBounds_TripsTimelineImmutables verifies the timeline
// immutables sidecar bound trips when entries accumulate past 2× frames.
func TestCheckComponentBounds_TripsTimelineImmutables(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// Build a minimal scene so g.drum.Rows is non-empty (the bound iterates
	// over rows) and g.timeline is wired up.
	buildSoakScene(t, g, 1 /* rows */, 4 /* nodes */)

	const frames = 100
	// Inflate by recording many import-kind commits at fresh abs indices —
	// each goes into immutables[0]. We use a windowLen/cap large enough to
	// keep ring growth uncapped; the test only inspects immutables.
	for i := 0; i < frames*3; i++ {
		g.timeline.RecordCommitKind(0, i, true, model.NodeTypeRegular, timeline.CommitKindPlayback, 1, 1)
	}

	fails := checkComponentBounds(g, soakScenario{name: "synthetic"}, frames)
	if !anyContains(fails, "timeline[row=0] immutables") {
		t.Fatalf("expected timeline.immutables bound to trip; got fails=%v", fails)
	}
}

// TestCheckComponentBounds_PassesUnderClean confirms that a freshly built
// game with no synthetic inflation does not trip any per-component bound.
// Guards against accidentally setting bounds too tight.
func TestCheckComponentBounds_PassesUnderClean(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	buildSoakScene(t, g, 2, 4)

	fails := checkComponentBounds(g, soakScenario{name: "synthetic-clean"}, 8400)
	if len(fails) > 0 {
		t.Fatalf("expected no bound failures on clean game; got: %v", fails)
	}
}

func anyContains(s []string, sub string) bool {
	for _, x := range s {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}
