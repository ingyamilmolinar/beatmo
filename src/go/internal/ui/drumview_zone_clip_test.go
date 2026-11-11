//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestDrumViewZoneClipRectsMatchWidgetRects asserts that each zone is clipped
// to its own WidgetBoard cell — not to the entire DrumView bounds. This is
// the layout-isolation invariant that prevents the timeline zone (cells)
// from bleeding across the rack column (instrument labels) and similar
// cross-widget overdraw.
func TestDrumViewZoneClipRectsMatchWidgetRects(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	cases := []struct {
		zone Zone
		kind WidgetKind
		name string
	}{
		{dv.transportZone, WidgetTransport, "transport"},
		{dv.rowRackZone, WidgetRack, "row-rack"},
		{dv.timelineZone, WidgetTimeline, "timeline"},
		{dv.eqPanelZone, WidgetWave, "eq-panel"},
	}
	for _, c := range cases {
		want := dv.widgetRects[c.kind]
		if want.Empty() {
			t.Fatalf("widgetRects[%s] is empty; precondition for clip test failed", c.kind)
		}
		got := dv.zoneClipRect(c.zone)
		if got != want {
			t.Errorf("%s zone clip rect = %v, want widgetRects[%s] = %v",
				c.name, got, c.kind, want)
		}
	}
}

// TestDrumViewZoneClipRectFallsBackToBoundsWhenWidgetEmpty ensures the helper
// degrades gracefully when a widget rect is unavailable (degenerate layouts
// or unknown zones). This guards against silently producing empty SubImages
// in tests that don't initialise the WidgetBoard fully.
func TestDrumViewZoneClipRectFallsBackToBoundsWhenWidgetEmpty(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	// Save and clear the timeline widget rect to simulate a degenerate layout.
	saved := dv.widgetRects[WidgetTimeline]
	t.Cleanup(func() { dv.widgetRects[WidgetTimeline] = saved })
	dv.widgetRects[WidgetTimeline] = image.Rectangle{}

	got := dv.zoneClipRect(dv.timelineZone)
	if got != dv.Bounds {
		t.Errorf("expected fallback to dv.Bounds when widget rect empty: got=%v bounds=%v",
			got, dv.Bounds)
	}
}

// TestDrumViewTimelineDrawStaysWithinTimelineRect runs a full DrumView.Draw
// and asserts that no drawRect call issued during the timeline zone's draw
// callback paints a row-stripe color into the rack column. This reproduces
// the user-visible "yellow row band bleeding behind instrument labels" bug.
func TestDrumViewTimelineDrawStaysWithinTimelineRect(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	rack := dv.widgetRects[WidgetRack]
	timeline := dv.widgetRects[WidgetTimeline]
	if rack.Empty() || timeline.Empty() {
		t.Fatalf("preconditions: rack=%v timeline=%v", rack, timeline)
	}

	// Stripe colors used by drawRowsDirect for row backgrounds.
	stripeEven := genColorDrumStripeEven
	stripeOdd := genColorDrumStripeOdd
	rh := dv.rowHeight()

	rec := &drawCallRecorder{}
	dst := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	rec.record(t, func() {
		dv.Draw(dst, nil, 0, nil, 0)
	})

	for _, c := range rec.calls {
		if c.Kind != drawCallRect {
			continue
		}
		if c.Color != stripeEven && c.Color != stripeOdd {
			continue
		}
		// The drum-stripe tokens now alias the background/surface-1 tokens by
		// design — row stripes are dark magenta-violet so empty cells recede
		// into the background (DESIGN.md: drum-stripe-even == surface-1,
		// drum-stripe-odd == background). That means legitimate full-zone
		// background fills share the stripe RGBA and must NOT count as bleed.
		// A real per-row stripe is exactly one rowHeight tall; zone/area
		// backgrounds are much taller. Skip anything taller than a single row.
		if c.Rect.Dy() > rh {
			continue
		}
		// Row stripes drawn into the offscreen row-sprite buffer use
		// sprite-internal coordinates (origin (0,0)) and never reach the
		// screen except via the rowsLayer DrawImage blit. They appear in
		// the recorder but don't represent screen-space bleed. Filter
		// those out by requiring the call to overlap the rack widget rect
		// — that is the user-visible bleed symptom in screen coordinates.
		if c.Rect.Overlaps(rack) {
			t.Errorf("row stripe drawRect overlaps rack column (bleed): rect=%v color=%v rack=%v",
				c.Rect, c.Color, rack)
		}
	}
	_ = timeline
}
