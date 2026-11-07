//go:build test

package ui

import (
	"strings"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Reachable inline on the timeline header, not through the overflow kebab.
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
	if tr := trackTouchRect(t, dv); tr.Dx() < TouchMinTarget() || tr.Dy() < TouchMinTarget() {
		t.Errorf("track chip touch target %v below touch target %d", tr, TouchMinTarget())
	}
	// The lock is the band's LEFT anchor, left of the beat counter, on both platforms.
	bc := dv.beatCounterRect
	if r.Max.X > bc.Min.X+2 {
		t.Errorf("track chip %v should sit left of the beat counter rect %v", r, bc)
	}
	if r.Min.X < dv.timelineRect.Min.X-TouchMinTarget()-8 {
		t.Errorf("track chip %v sits implausibly far left of the band", r)
	}
}

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

	// The handler directly: geometry is covered above.
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

// The inline chip is the one place the toggle lives.
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
