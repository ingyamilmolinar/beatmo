//go:build test

package ui

import (
	"image"
	"testing"

	gamelog "github.com/ingyamilmolinar/beatmo/internal/log"
)

// One inverse of bottomNavModeList, so a reordered list can never desync a hand-written switch.
func TestSegmentIndexForViewModeMatchesCanonicalList(t *testing.T) {
	for i, m := range bottomNavModeList {
		if got := segmentIndexForViewMode(m); got != i {
			t.Errorf("segmentIndexForViewMode(%v) = %d, want %d", m, got, i)
		}
		// Round-trip: the index must map back to the same mode.
		if bottomNavModeList[segmentIndexForViewMode(m)] != m {
			t.Errorf("segmentIndexForViewMode is not the inverse of bottomNavModeList for %v", m)
		}
	}
}

// Derived from the same list as the click handler, so segment → mode and mode → segment cannot drift.
func TestSetViewModeActivatesCanonicalSegment(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	withSmallScreen(t, true)

	logger := gamelog.New(testLogOutput(), gamelog.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 700), nil, logger)
	if dv.viewSwitchSegmented == nil {
		t.Fatal("segmented control not constructed")
	}

	for i, m := range bottomNavModeList {
		dv.setViewMode(m)
		if got := dv.viewSwitchSegmented.Active(); got != i {
			t.Errorf("after setViewMode(%v): segment Active() = %d, want %d", m, got, i)
		}
	}
}
