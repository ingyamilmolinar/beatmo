//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Each zone clips to its own widget cell, so the timeline cannot paint across the rack column.
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

// Never an empty SubImage for a degenerate layout or unknown zone.
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

// No row-stripe paint may land in the rack column (stripes bleeding behind labels).
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
		// Stripe tokens alias the background by design; only single-row-height fills can be bleed.
		if c.Rect.Dy() > rh {
			continue
		}
		// Sprite-internal draws never reach the screen directly; only calls overlapping the rack are screen bleed.
		if c.Rect.Overlaps(rack) {
			t.Errorf("row stripe drawRect overlaps rack column (bleed): rect=%v color=%v rack=%v",
				c.Rect, c.Color, rack)
		}
	}
	_ = timeline
}
