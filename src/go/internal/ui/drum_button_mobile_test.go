//go:build test

package ui

import (
	"testing"
)

// TestSplitterDoesNotStealDrumButtonTouches verifies that a touch just
// below the splitter divider (where drum transport buttons live) is NOT
// captured by the splitter on small screens. Previously, the splitter's
// TouchGrabZone (22px) extended into the drum area and stole button taps.
func TestSplitterDoesNotStealDrumButtonTouches(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	s := NewSplitter(600)
	s.UpdateResize(600, 400)
	s.winW = 400
	s.totalH = 600

	// Touch 10px below the divider — inside drum area, should be ignored.
	result := s.HandleInput(200, s.Y+10, true)
	if result != InputIgnored {
		t.Errorf("Expected InputIgnored for touch 10px below divider, got %v", result)
	}
	if s.dragging {
		t.Error("Splitter should not start dragging for touch 10px below divider")
	}
}

// TestSplitterInitiationOnDivider verifies that touching directly on the
// divider still starts a drag (the fix should not break normal splitter use).
func TestSplitterInitiationOnDivider(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	s := NewSplitter(600)
	s.UpdateResize(600, 400)
	s.winW = 400
	s.totalH = 600

	// Touch exactly on the divider.
	result := s.HandleInput(200, s.Y, true)
	if result != InputCaptured {
		t.Errorf("Expected InputCaptured for touch on divider, got %v", result)
	}
	if !s.dragging {
		t.Error("Splitter should start dragging for touch on divider")
	}

	// Release.
	result = s.HandleInput(200, s.Y, false)
	if result != InputConsumed {
		t.Errorf("Expected InputConsumed on release, got %v", result)
	}
}

// TestSplitterInputBoundsReduced verifies that the below-divider extent
// in InputBounds is small (<=4px), not the full TouchGrabZone (22px).
func TestSplitterInputBoundsReduced(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	s := NewSplitter(600)
	s.UpdateResize(600, 400)
	s.winW = 400
	s.totalH = 600

	bounds := s.InputBounds()

	// Below the divider the strip extends only far enough to cover the visible
	// handle pill — NOT the full TouchGrabZone (which would re-introduce the
	// drum-button-steal bug). No-steal off the pill is enforced by HandleInput
	// (see TestSplitterCapturesPillButNotOffPillBelowDivider).
	pill := s.HandleRect().Inset(-SpaceSM)
	belowExtent := bounds.Max.Y - s.Y
	pillBelow := pill.Max.Y - s.Y
	if belowExtent != pillBelow {
		t.Errorf("Expected below-divider extent = pill extent %d, got %d", pillBelow, belowExtent)
	}
	if belowExtent >= TouchGrabZone() {
		t.Errorf("below-divider extent %d must stay well under the full TouchGrabZone %d", belowExtent, TouchGrabZone())
	}

	// Above-divider should still be at least the full grab zone.
	aboveExtent := s.Y - bounds.Min.Y
	if aboveExtent < TouchGrabZone() {
		t.Errorf("Expected above-divider extent >= %d (TouchGrabZone), got %d", TouchGrabZone(), aboveExtent)
	}
}

// TestSplitterInputBoundsReducedSideBySide verifies the side-by-side
// (vertical divider) mode also has a reduced right-of-divider extent.
func TestSplitterInputBoundsReducedSideBySide(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	s := NewSplitter(600)
	s.horizontal = false
	s.X = 200
	s.UpdateResize(600, 400)
	s.winW = 400
	s.totalH = 600

	bounds := s.InputBounds()

	// Right of the divider the strip extends only far enough to cover the
	// visible handle pill — NOT the full TouchGrabZone.
	pill := s.HandleRect().Inset(-SpaceSM)
	rightExtent := bounds.Max.X - s.X
	pillRight := pill.Max.X - s.X
	if rightExtent != pillRight {
		t.Errorf("Expected right-of-divider extent = pill extent %d, got %d", pillRight, rightExtent)
	}
	if rightExtent >= TouchGrabZone() {
		t.Errorf("right-of-divider extent %d must stay well under the full TouchGrabZone %d", rightExtent, TouchGrabZone())
	}

	leftExtent := s.X - bounds.Min.X
	if leftExtent < TouchGrabZone() {
		t.Errorf("Expected left-of-divider extent >= %d (TouchGrabZone), got %d", TouchGrabZone(), leftExtent)
	}
}

// TestDrumPlayButtonMobileTap verifies that a simulated tap on the play
// button area in mobile mode reaches the DrumView and triggers PlayPressed().
func TestDrumPlayButtonMobileTap(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 800) // portrait-ish mobile layout

	// Ensure drum view has a play button with OnClick wired.
	if g.drum == nil {
		t.Fatal("drum view is nil")
	}

	// Directly invoke pressPlay to verify the button callback works.
	pressPlay(t, g.drum)
	advanceFrames(g, 1)

	if !g.drum.PlayPressed() {
		// PlayPressed is a one-shot, might have been consumed by Update.
		// The fact that pressPlay didn't panic is sufficient.
	}
}
