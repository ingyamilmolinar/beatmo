//go:build test

package ui

import (
	"image"
	"testing"

	gamelog "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestSegmentIndexForViewModeMatchesCanonicalList pins the single inverse of
// bottomNavModeList: segmentIndexForViewMode(m) must return m's position in the
// canonical segment list (and -1 for a mode with no segment). This replaces the
// hand-maintained SetActive(0..7) switch in setViewMode, which silently desyncs
// from bottomNavModeList if the list is reordered.
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

// TestSetViewModeActivatesCanonicalSegment is the integration guard: switching
// to any view mode must light up the matching segment, derived from the same
// canonical list as the click handler — so the forward (segment→mode) and
// reverse (mode→segment) mappings can never drift apart.
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
