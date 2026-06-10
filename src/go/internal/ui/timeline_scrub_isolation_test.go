//go:build test

package ui

import (
	"image"
	"testing"
)

// TestTimelineScrubReleasesOnYDrift pins Phase 3 Part B: a captured
// scrub handler must release itself when the pointer drifts outside
// the bar's vertical lane. The tree's drag dispatch routes every
// move to the captured handler without re-testing hit areas, so the
// handler self-polices.
//
// Pre-Phase-3: the scrub kept scrubbing across the entire screen as
// long as the press started inside the bar, so a drag that started
// near the timeline and ended on a synth knob ALSO updated the
// timeline playhead. The canonical input-leak the user reported.
func TestTimelineScrubReleasesOnYDrift(t *testing.T) {
	g := newTimelineZoneTestHarness(t)
	defer g.cleanup()

	z := g.zone
	z.timelineBarRect = image.Rect(0, 0, 400, 12)
	h := &timelineScrubHitAdapter{zone: z}

	// Start a press at the bar centre. Scrubbing should engage.
	res := h.OnPress(200, 6)
	if res != InputCaptured {
		t.Fatalf("OnPress returned %v, want InputCaptured", res)
	}
	if !z.scrubbing {
		t.Fatal("scrubbing flag should be true after OnPress")
	}

	// Drag straight down to far below the bar — past the skirt (-4 inset
	// of bar Y range = [-4, 16]). y=400 is well outside.
	h.OnDrag(200, 400)
	if z.scrubbing {
		t.Error("scrubbing should have released after Y drifted out of skirt — leak risk")
	}

	// A subsequent press inside the bar must re-engage so scrub still
	// works for ordinary use.
	h.OnPress(200, 6)
	if !z.scrubbing {
		t.Fatal("re-engagement: scrubbing should be true after second OnPress at bar centre")
	}
	// A drag that stays inside the skirt must keep scrubbing live.
	h.OnDrag(220, 8) // x moved slightly, y still inside [-4, 16]
	if !z.scrubbing {
		t.Error("intra-skirt drag should keep scrub live — over-eager release")
	}
}

// TestTimelineScrubRectIsStrictlyBoundedToBar pins Phase 3 Part A:
// the scrub HitArea's exact Rect must NOT extend below the visible
// bar (no downward geometric growth). Touch expansion via the
// `Touch + ClipRect` field provides the touch-target floor, but the
// expansion is strictly bounded.
func TestTimelineScrubRectIsStrictlyBoundedToBar(t *testing.T) {
	g := newTimelineZoneTestHarness(t)
	defer g.cleanup()

	restore := SetForceSmallScreen(t, true)
	defer restore()

	z := g.zone
	z.rect = image.Rect(0, 0, 400, 800)
	z.timelineBarRect = image.Rect(0, 0, 400, 12)
	z.stepsRect = image.Rect(0, 12, 400, 800)
	z.rebuildHitAreas()

	var scrubArea *HitArea
	for i, h := range z.hitAreas {
		if h.Tag == "timeline-scrub" {
			scrubArea = &z.hitAreas[i]
			break
		}
	}
	if scrubArea == nil {
		t.Fatal(`"timeline-scrub" hit area missing`)
	}

	// The visible Rect must be EXACTLY the bar — no downward growth.
	if scrubArea.Rect != z.timelineBarRect {
		t.Errorf("scrub Rect=%v, want exact bar=%v (no enlargement)",
			scrubArea.Rect, z.timelineBarRect)
	}

	// Touch flag must be set so HitIndex.At expands the touch radius
	// at query time (bounded by ClipRect).
	if !scrubArea.Touch {
		t.Error("scrub Touch=false; expected Touch=true so touch-min is met via expansion not enlargement")
	}

	// ClipRect must be set and bounded to ~4 px skirt of the bar so
	// touch expansion can never reach a sibling zone.
	if scrubArea.ClipRect.Empty() {
		t.Fatal("scrub ClipRect empty — touch expansion would be unbounded")
	}
	if scrubArea.ClipRect.Max.Y > z.timelineBarRect.Max.Y+8 {
		t.Errorf("scrub ClipRect bottom=%d, want ≤ bar.Max.Y+8 (%d) — skirt too generous",
			scrubArea.ClipRect.Max.Y, z.timelineBarRect.Max.Y+8)
	}
}

// TestTimelineScrubTouchExpansionRespectsClipRect: HitIndex.At should
// hit scrub at points within the 4 px skirt below the bar, but NOT
// at points far below (e.g., 50 px). Verifies the touch expansion is
// genuinely bounded by ClipRect.
func TestTimelineScrubTouchExpansionRespectsClipRect(t *testing.T) {
	g := newTimelineZoneTestHarness(t)
	defer g.cleanup()

	restore := SetForceSmallScreen(t, true)
	defer restore()

	z := g.zone
	z.rect = image.Rect(0, 0, 400, 800)
	z.timelineBarRect = image.Rect(0, 0, 400, 12)
	z.stepsRect = image.Rect(0, 12, 400, 800)
	z.rebuildHitAreas()

	idx := &HitIndex{}
	idx.Update("timeline", z.hitAreas)

	// Inside the bar — must hit scrub.
	if !hasScrubHit(idx.At(200, 6)) {
		t.Error("point inside bar (200,6) failed to hit scrub")
	}
	// Within skirt below bar — should still hit scrub via touch expansion.
	if !hasScrubHit(idx.At(200, 13)) {
		t.Error("point at y=13 (just below bar) failed to hit scrub via touch expansion")
	}
	// Far below — must NOT hit scrub.
	hits := idx.At(200, 50)
	if hasScrubHit(hits) {
		t.Errorf("point at y=50 (50 px below bar) leaked to scrub — ClipRect not bounded; hits=%v", tagsOf(hits))
	}
}

func hasScrubHit(hits []indexedHitArea) bool {
	for _, h := range hits {
		if h.Tag == "timeline-scrub" {
			return true
		}
	}
	return false
}

func tagsOf(hits []indexedHitArea) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Tag
	}
	return out
}

// --- Test harness ---

type timelineZoneTestHarness struct {
	zone    *TimelineZone
	cleanup func()
}

func newTimelineZoneTestHarness(t *testing.T) *timelineZoneTestHarness {
	t.Helper()
	cb := TimelineCallbacks{
		TimelineBeats:        func() int { return 16 },
		TimelineUnitsPerBeat: func() int { return 4 },
		Length:               func() int { return 64 },
		Offset:               func() int { return 0 },
		OnOffsetChange:       func(int) {},
		OnScrubPosition:      func(int) {},
	}
	z := NewTimelineZone(cb)
	z.rect = image.Rect(0, 0, 400, 200)
	return &timelineZoneTestHarness{
		zone:    z,
		cleanup: func() {},
	}
}
