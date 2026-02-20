//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newDrumViewForTrackTest(t *testing.T, mobile bool) *DrumView {
	t.Helper()
	w, h := 1280, 720
	if mobile {
		w, h = 390, 844
	}
	dv := NewDrumView(image.Rect(0, 0, w, h), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), CellTypes: make([]model.NodeType, 8), Volume: 1.0},
	}
	dv.Length = 8
	dv.recalcButtons()
	dv.calcLayout()
	return dv
}

// TestTrackButtonHidden_Mobile verifies the Track button is hidden on mobile.
func TestTrackButtonHidden_Mobile(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForTrackTest(t, true)
	btn := dv.TrackBtnForTest()
	r := btn.Rect()

	if !r.Empty() {
		t.Fatalf("Track button should be hidden on mobile, got %v", r)
	}
}

// TestTrackButtonToggle_Mobile verifies the track button is icon-only on mobile.
func TestTrackButtonToggle_Mobile(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForTrackTest(t, true)
	btn := dv.TrackBtnForTest()

	// Track button is icon-only (no text).
	if btn.Text != "" {
		t.Fatalf("expected empty text for icon-only track button, got %q", btn.Text)
	}
	if btn.Icon != "track" {
		t.Fatalf("expected icon 'track', got %q", btn.Icon)
	}

	// Toggle: follow=false should switch icon to "track-off".
	dv.SetFollow(false)
	if btn.Text != "" {
		t.Fatalf("expected empty text after SetFollow(false), got %q", btn.Text)
	}
	if btn.Icon != "track-off" {
		t.Fatalf("expected icon 'track-off' after SetFollow(false), got %q", btn.Icon)
	}

	// Toggle back: follow=true should restore "track" icon.
	dv.SetFollow(true)
	if btn.Text != "" {
		t.Fatalf("expected empty text after SetFollow(true), got %q", btn.Text)
	}
	if btn.Icon != "track" {
		t.Fatalf("expected icon 'track' after SetFollow(true), got %q", btn.Icon)
	}
}

// TestTrackButtonNotInOverflowMenu verifies that Track is removed from the
// overflow menu items.
func TestTrackButtonNotInOverflowMenu(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForTrackTest(t, true)
	items := dv.OverflowItemsForTest()
	for _, item := range items {
		if item.label == "Track" {
			t.Fatal("Track should not appear in overflow menu items")
		}
	}
}

// TestTrackButtonHiddenNotInTransportRow_Mobile verifies the Track button is
// hidden on mobile (not in any transport row).
func TestTrackButtonHiddenNotInTransportRow_Mobile(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForTrackTest(t, true)
	btn := dv.TrackBtnForTest()
	r := btn.Rect()

	if !r.Empty() {
		t.Fatalf("Track button should be hidden on mobile, got %v", r)
	}
}

// TestTrackButtonDesktopUnchanged verifies that on desktop, the Track button
// is icon-only in the single transport row.
func TestTrackButtonDesktopUnchanged(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, false)

	dv := newDrumViewForTrackTest(t, false)
	btn := dv.TrackBtnForTest()

	// Desktop: button should be icon-only
	if btn.Text != "" {
		t.Fatalf("expected empty text for icon-only track button, got %q", btn.Text)
	}
	if btn.Icon != "track" {
		t.Fatalf("expected icon 'track', got %q", btn.Icon)
	}

	// Desktop: button rect should NOT be empty (it's in the single row)
	if btn.Rect().Empty() {
		t.Fatal("Track button rect is empty on desktop — expected in transport row")
	}

	// Toggle should not change text (icon-only).
	dv.SetFollow(false)
	if btn.Text != "" {
		t.Fatalf("expected empty text after SetFollow(false), got %q", btn.Text)
	}

	dv.SetFollow(true)
	if btn.Text != "" {
		t.Fatalf("expected empty text after SetFollow(true), got %q", btn.Text)
	}
}
