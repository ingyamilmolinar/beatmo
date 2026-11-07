//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// The Rect is the visible bar, Touch is set and ClipRect bounds the expansion: a point in the skirt hits, one
// far below does not. A physically enlarged rect leaked into sibling zones.
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
	// Just below the bar, inside ClipRect.
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

// Exactly the visible bar, with no downward growth, so it never overlaps the length/track buttons below.
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
	if !scrubArea.Rect.Intersect(z.stepsRect).Empty() {
		t.Errorf("scrub Rect %v overlaps stepsRect %v — bar leaked into rows region",
			scrubArea.Rect, z.stepsRect)
	}
}

// During the post-pinch multi-touch cooldown, so stray pinch fingertips never start a scrub.
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
	// RecentMultiTouch() stays true until the cooldown ticks to zero.
	prevCooldown := globalTouchState.multiTouchCooldown
	globalTouchState.multiTouchCooldown = multiTouchCooldownFrames
	t.Cleanup(func() { globalTouchState.multiTouchCooldown = prevCooldown })

	adapter := &timelineScrubHitAdapter{zone: z}
	scrubMid := image.Pt(z.timelineBarRect.Min.X+z.timelineBarRect.Dx()/2,
		z.timelineBarRect.Min.Y+z.timelineBarRect.Dy()/2)

	// Before OnPress, or the no-buzz check is vacuous.
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

// 8 ms on mobile so the user knows the scrub engaged; after the multi-touch gate, so a suppressed press is
// silent.
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

// At most one seek per gesture, never one per drag frame, while the ghost beat tracks the drag monotonically.
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

	// Wrapped, so production behaviour is kept.
	var seeks []int
	prev := z.callbacks.OnSeekToStep
	z.callbacks.OnSeekToStep = func(step int, origin hooks.SeekOrigin) {
		seeks = append(seeks, step)
		if prev != nil {
			prev(step, origin)
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
	ghostAfterPress := z.scrubGhostBeat

	adapter.OnDrag((startX+endX)/2, y)
	ghostAfterMidDrag := z.scrubGhostBeat

	adapter.OnDrag(endX, y)
	ghostAfterEndDrag := z.scrubGhostBeat

	adapter.OnRelease(endX, y)

	// The press seeks nothing; the full-width drag commits exactly one, on release.
	if len(seeks) != 1 {
		t.Fatalf("expected exactly 1 OnSeekToStep call (release only), got %d: %v", len(seeks), seeks)
	}
	if seeks[0] <= 0 {
		t.Fatalf("expected release seek near the right edge (left edge -> right edge drag); saw %v", seeks)
	}

	// Drag advances the position at the preview layer.
	if ghostAfterMidDrag <= ghostAfterPress {
		t.Fatalf("expected ghost to advance past press position at mid-drag; press=%d mid=%d", ghostAfterPress, ghostAfterMidDrag)
	}
	if ghostAfterEndDrag <= ghostAfterMidDrag {
		t.Fatalf("expected ghost to advance past mid-drag at end-drag; mid=%d end=%d", ghostAfterMidDrag, ghostAfterEndDrag)
	}
	if z.scrubbing {
		t.Fatalf("expected scrubbing=false after OnRelease, got true")
	}
}
