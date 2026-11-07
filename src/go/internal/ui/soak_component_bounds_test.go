//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// Long soaks rely on this to catch a removed or bypassed drop-oldest cap.
func TestCheckComponentBounds_TripsParityAudio(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// Only the length is asserted.
	g.parityAudio = make([]parityAudioEvent, 1025)

	fails := checkComponentBounds(g, soakScenario{name: "synthetic"}, 8400)
	if !anyContains(fails, "parityAudio") {
		t.Fatalf("expected parityAudio bound to trip; got fails=%v", fails)
	}
}

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

func TestCheckComponentBounds_TripsTimelineImmutables(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// The bound iterates over rows, and needs g.timeline wired.
	buildSoakScene(t, g, 1 /* rows */, 4 /* nodes */)

	const frames = 100
	// A large window keeps the ring uncapped; only immutables are inspected.
	for i := 0; i < frames*3; i++ {
		g.timeline.RecordCommitKind(0, i, true, model.NodeTypeRegular, timeline.CommitKindPlayback, 1, 1)
	}

	fails := checkComponentBounds(g, soakScenario{name: "synthetic"}, frames)
	if !anyContains(fails, "timeline[row=0] immutables") {
		t.Fatalf("expected timeline.immutables bound to trip; got fails=%v", fails)
	}
}

// Bounds that trip on a clean game are set too tight.
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
