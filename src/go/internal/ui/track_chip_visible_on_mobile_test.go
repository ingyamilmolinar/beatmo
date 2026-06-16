//go:build test

package ui

import (
	"strings"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestTrackChip_VisibleOnMobile verifies the Free/Track follow toggle is
// reachable inline on the timeline ruler header on mobile (Theme 2 of the
// mobile UI consistency pass). Before this change the toggle was only
// reachable through the overflow kebab.
func TestTrackChip_VisibleOnMobile(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	tb := dv.trackBtn()
	if tb == nil {
		t.Fatal("trackBtn nil")
	}
	r := tb.Rect()
	if r.Empty() {
		t.Fatalf("track chip rect empty on mobile; want non-empty inside the timeline ruler header")
	}
	// Chip must hit the touch-target floor.
	if r.Dx() < TouchMinTarget() || r.Dy() < TouchMinTarget() {
		t.Errorf("track chip %v below touch target %d", r, TouchMinTarget())
	}
	// Unified layout: the track/lock chip is the LEFT anchor of the band, so
	// it sits to the LEFT of the beat counter (which itself sits left of the
	// notification area). Previously the chip was on the right edge of the
	// beat counter; the notification redesign moved it left on both platforms.
	bc := dv.beatCounterRect
	if r.Max.X > bc.Min.X+2 {
		t.Errorf("track chip %v should sit left of the beat counter rect %v", r, bc)
	}
	if r.Min.X < dv.timelineRect.Min.X-TouchMinTarget()-8 {
		t.Errorf("track chip %v sits implausibly far left of the band", r)
	}
}

// TestTrackChip_TogglesFollowState verifies tapping the chip flips
// FollowPlayback() and the icon swaps IconTrack <-> IconTrackOff.
func TestTrackChip_TogglesFollowState(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	if !dv.FollowPlayback() {
		t.Fatal("expected default FollowPlayback() == true (seed state)")
	}
	if got := dv.trackBtn().Icon; got != string(IconTrack) {
		t.Errorf("initial Track icon = %q, want %q", got, string(IconTrack))
	}

	// Fire the click handler directly — geometry is covered by the
	// Visible test above; this exercises the toggle behavior.
	if cb := dv.trackBtn().OnClick; cb != nil {
		cb()
	}
	advanceFrames(g, 1)

	if dv.FollowPlayback() {
		t.Error("after toggle: FollowPlayback() still true")
	}
	if got := dv.trackBtn().Icon; got != string(IconTrackOff) {
		t.Errorf("after toggle: Track icon = %q, want %q", got, string(IconTrackOff))
	}
}

// TestTrackEntry_RemovedFromOverflowMenu verifies the duplicate Track
// entry no longer appears in the overflow kebab now that the chip is
// inline (single source of truth for the toggle).
func TestTrackEntry_RemovedFromOverflowMenu(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	for _, item := range dv.OverflowItemsForTest() {
		if strings.EqualFold(item.label, "Track") {
			t.Errorf("overflow menu still contains Track entry; should be inline-only")
		}
	}
}
