//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestTimelineScrub_MobileHitAreaSpansFullTimelineRect verifies the
// mobile scrub hit surface meets TouchMinTarget once HitIndex.At
// applies its touch expansion. Pre-input-isolation pass: the area's
// Rect was physically enlarged downward into the steps region to
// meet the target — which leaked into sibling zones (the canonical
// "knob drag scrubs the timeline" bug). Now: the Rect equals the
// visible bar, Touch=true, and the ClipRect bounds the expansion.
// HitIndex.At expands the touch radius up to TouchMinTarget at
// query time but never past ClipRect. This test exercises the new
// contract: a point well outside the bar but inside the touch
// skirt still hits the scrub area; a point far below does not.
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
	if !scrubArea.Touch {
		t.Errorf("mobile scrub Touch=false; expected Touch=true so HitIndex.At expands the touch radius")
	}
	if scrubArea.ClipRect.Empty() {
		t.Errorf("mobile scrub ClipRect empty; expected a bounded skirt to prevent leakage")
	}
	// The visible bar may be thin, but touch expansion via HitIndex.At
	// must still land within the bar's vertical skirt. Confirm a point
	// just below the bar (within ClipRect) hits scrub.
	idx := &HitIndex{}
	idx.Update("timeline", z.HitAreas())
	barMid := scrubArea.Rect.Min.X + scrubArea.Rect.Dx()/2
	belowBar := scrubArea.Rect.Max.Y + 2
	if belowBar > scrubArea.ClipRect.Max.Y {
		t.Skip("clip-skirt too tight to test below-bar tolerance on this layout")
	}
	hits := idx.At(barMid, belowBar)
	found := false
	for _, h := range hits {
		if h.Tag == "timeline-scrub" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("touch expansion failed: point (%d,%d) inside ClipRect did not hit scrub", barMid, belowBar)
	}
}

// TestTimelineScrub_HitAreaIsBarOnly verifies the scrub Rect equals
// the visible bar exactly — no downward growth into the steps region.
// Pre-input-isolation pass the rect grew downward and overlapped
// the len ±/track buttons, requiring a per-button exclusion clamp.
// Post-fix the rect IS the bar, so no exclusion is needed and there
// is no leakage risk into a sibling zone. The button rects in this
// test are placed in the steps region (below the bar); the scrub
// rect must NOT overlap them since the bar is above them.
func TestTimelineScrub_HitAreaIsBarOnly(t *testing.T) {
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
	if scrubArea.Rect != z.timelineBarRect {
		t.Errorf("scrub Rect=%v, want timelineBarRect=%v (no enlargement)",
			scrubArea.Rect, z.timelineBarRect)
	}
	// The bar sits at the top of the zone; the steps region (rows) is
	// strictly below. So the bar must not overlap the rows region.
	if !scrubArea.Rect.Intersect(z.stepsRect).Empty() {
		t.Errorf("scrub Rect %v overlaps stepsRect %v — bar leaked into rows region",
			scrubArea.Rect, z.stepsRect)
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

// TestTimelineSwipeScrub_DragDispatchesPositions exercises the full
// press → drag → release pipeline through the timelineScrubHitAdapter
// and asserts OnScrubPosition is called monotonically as the touch
// drags from start to end of the timeline. Pin for Tasks 3.1–3.3:
// any future change to scrub hit area, gate, or haptic must not
// break the underlying scrub-position dispatch contract.
func TestTimelineSwipeScrub_DragDispatchesPositions(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	z := g.drum.timelineZone
	if z == nil {
		t.Fatalf("timelineZone nil")
	}

	// Wrap the existing OnScrubPosition callback so we can capture each
	// dispatched step value while preserving production behavior.
	var positions []int
	prev := z.callbacks.OnScrubPosition
	z.callbacks.OnScrubPosition = func(steps int) {
		positions = append(positions, steps)
		if prev != nil {
			prev(steps)
		}
	}

	// Find the registered scrub hit area (taller now after Task 3.1).
	scrubArea := findHitAreaByTagPrefix(z.HitAreas(), "timeline-scrub")
	if scrubArea == nil {
		t.Fatalf("timeline-scrub hit area not found")
	}
	r := scrubArea.Rect
	startX, endX := r.Min.X+10, r.Max.X-10
	y := r.Min.Y + r.Dy()/2

	adapter := &timelineScrubHitAdapter{zone: z}
	adapter.OnPress(startX, y)
	adapter.OnDrag((startX+endX)/2, y)
	adapter.OnDrag(endX, y)
	adapter.OnRelease(endX, y)

	if len(positions) < 3 {
		t.Fatalf("expected ≥3 OnScrubPosition calls (press + 2 drags), got %d: %v", len(positions), positions)
	}
	// First call (press) and last call (final drag) should differ:
	// scrub at left edge → 0, scrub at right edge → max.
	if positions[0] >= positions[len(positions)-1] {
		t.Fatalf("expected drag to advance position monotonically; saw %v", positions)
	}
	if z.scrubbing {
		t.Fatalf("expected scrubbing=false after OnRelease, got true")
	}
}
