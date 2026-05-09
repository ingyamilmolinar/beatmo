//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestTimelineScrub_MobileHitAreaSpansFullTimelineRect verifies the
// mobile scrub hit surface is at least TouchMinTarget tall (B5 critique:
// the bar-only hit area was too narrow for thumb scrubs).
func TestTimelineScrub_MobileHitAreaSpansFullTimelineRect(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	z := g.drum.timelineZone
	if z == nil {
		t.Fatalf("timelineZone nil")
	}
	scrubArea := findHitAreaByTagPrefix(z.HitAreas(), "timeline-scrub")
	if scrubArea == nil {
		t.Fatalf("timeline-scrub hit area not found")
	}
	if scrubArea.Rect.Dy() < TouchMinTarget() {
		t.Fatalf("mobile scrub hit area height %d < TouchMinTarget %d", scrubArea.Rect.Dy(), TouchMinTarget())
	}
}

// TestTimelineScrub_HitAreaExcludesLenButtons verifies the enlarged
// scrub area does NOT swallow taps on the len ± buttons (when they
// exist on a given layout).
func TestTimelineScrub_HitAreaExcludesLenButtons(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	z := g.drum.timelineZone
	if z == nil {
		t.Fatalf("timelineZone nil")
	}
	scrubArea := findHitAreaByTagPrefix(z.HitAreas(), "timeline-scrub")
	if scrubArea == nil {
		t.Fatalf("timeline-scrub hit area not found")
	}
	if z.lenIncBtn != nil {
		if r := z.lenIncBtn.Rect(); !r.Empty() && !scrubArea.Rect.Intersect(r).Empty() {
			t.Errorf("scrub hit area %v overlaps lenIncBtn %v", scrubArea.Rect, r)
		}
	}
	if z.lenDecBtn != nil {
		if r := z.lenDecBtn.Rect(); !r.Empty() && !scrubArea.Rect.Intersect(r).Empty() {
			t.Errorf("scrub hit area %v overlaps lenDecBtn %v", scrubArea.Rect, r)
		}
	}
}

// TestTimelineScrub_PinchDoesNotStartScrub verifies that during the
// multi-touch cooldown (set by the global TouchState after a pinch
// gesture ends) a scrub press is suppressed. Without this gate the
// enlarged mobile scrub area (Task 3.1) would let stray fingertips
// from a pinch gesture fire false scrubs.
func TestTimelineScrub_PinchDoesNotStartScrub(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	z := g.drum.timelineZone
	if z == nil {
		t.Fatalf("timelineZone nil")
	}
	// Simulate a recent multi-touch by pinning the global cooldown
	// counter directly. RecentMultiTouch() will return true until the
	// cooldown ticks down to zero.
	prevCooldown := globalTouchState.multiTouchCooldown
	globalTouchState.multiTouchCooldown = multiTouchCooldownFrames
	t.Cleanup(func() { globalTouchState.multiTouchCooldown = prevCooldown })

	adapter := &timelineScrubHitAdapter{zone: z}
	scrubMid := image.Pt(z.timelineBarRect.Min.X+z.timelineBarRect.Dx()/2,
		z.timelineBarRect.Min.Y+z.timelineBarRect.Dy()/2)

	// Install the haptic spy BEFORE OnPress so the no-buzz assertion
	// below is meaningful — installing it after the call would make
	// len(*captured)==0 vacuously true.
	captured := withCapturedHaptics(t)
	res := adapter.OnPress(scrubMid.X, scrubMid.Y)

	if z.scrubbing {
		t.Fatalf("scrubbing should remain false during multi-touch cooldown")
	}
	if res == InputCaptured || res == InputConsumed {
		t.Fatalf("OnPress should ignore press during multi-touch cooldown, got %v", res)
	}
	if len(*captured) != 0 {
		t.Fatalf("expected no haptic when press is suppressed by multi-touch gate; got %v", *captured)
	}
}

// TestTimelineScrub_OnPressFiresHaptic verifies an 8ms vibration
// fires when scrub begins on mobile. The cue is critical for users to
// know they've engaged the scrub gesture (the new enlarged hit area
// from Task 3.1 makes the boundary less obvious without it). Fires
// after the multi-touch gate so suppressed presses don't buzz.
func TestTimelineScrub_OnPressFiresHaptic(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	captured := withCapturedHaptics(t)

	z := g.drum.timelineZone
	if z == nil {
		t.Fatalf("timelineZone nil")
	}
	adapter := &timelineScrubHitAdapter{zone: z}
	mid := image.Pt(z.timelineBarRect.Min.X+z.timelineBarRect.Dx()/2,
		z.timelineBarRect.Min.Y+z.timelineBarRect.Dy()/2)
	adapter.OnPress(mid.X, mid.Y)

	if len(*captured) != 1 || (*captured)[0] != 8 {
		t.Fatalf("expected one 8 ms haptic on scrub start; got %v", *captured)
	}
}
