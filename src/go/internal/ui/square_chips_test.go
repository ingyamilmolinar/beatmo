//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// roundedCallsInside captures drawRoundedRect calls made during fn() that land
// inside `inside` and use a visible corner radius (>= 1). A square chip makes
// none. Used by the square-chip tests below.
func roundedCallsInside(t *testing.T, inside image.Rectangle, fn func()) []roundedDraw {
	t.Helper()
	var rounded []roundedDraw
	orig := drawRoundedRect
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		if radius >= 1 && !r.Empty() && r.In(inside) {
			rounded = append(rounded, roundedDraw{Rect: r, Color: c, Radius: radius, Filled: filled})
		}
		orig(dst, r, c, radius, filled)
	}
	t.Cleanup(func() { drawRoundedRect = orig })
	fn()
	return rounded
}

// TestBeatCounterChipIsSquare verifies the beat/timer counter chip is drawn
// with square corners (a plain rect), not a rounded pill — the user finds the
// rounding ugly. A filled colSurface1 chip must still be drawn (so it remains
// a real container), just with no corner radius.
func TestBeatCounterChipIsSquare(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 200, 24)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	rounded := roundedCallsInside(t, beatRect, func() { z.drawBeatCounter(dst, 0) })
	if len(rounded) > 0 {
		t.Fatalf("beat counter chip drawn with rounded corners (radius>=1): %+v — should be square", rounded)
	}

	// And a square colSurface1 fill must still be present.
	fills := collectFilledRects(t, func() { z.drawBeatCounter(dst, 0) })
	if rectsWithColorInside(fills, beatRect, colSurface1) == 0 {
		t.Fatalf("no square colSurface1 fill drawn inside beat counter rect %v", beatRect)
	}
}

// newNotifTestZone builds a TimelineZone with a notification area for the
// square-chip test.
func newNotifTestZone(notif image.Rectangle) *TimelineZone {
	return NewTimelineZone(TimelineCallbacks{
		NotifRect:   func() image.Rectangle { return notif },
		NotifLatest: func() (string, bool, bool) { return "hello", false, true },
		Frame:       func() int64 { return 0 },
	})
}

// TestNotifAreaIsSquare verifies the dedicated notification area is drawn with
// square corners (a plain rect), matching the squared beat-counter chip.
func TestNotifAreaIsSquare(t *testing.T) {
	assertDefaultParityState(t)
	notifRect := image.Rect(40, 0, 240, 24)
	z := newNotifTestZone(notifRect)
	dst := ebiten.NewImage(notifRect.Max.X+16, notifRect.Max.Y+16)

	rounded := roundedCallsInside(t, notifRect, func() { z.drawNotifArea(dst) })
	if len(rounded) > 0 {
		t.Fatalf("notification area drawn with rounded corners (radius>=1): %+v — should be square", rounded)
	}

	fills := collectFilledRects(t, func() { z.drawNotifArea(dst) })
	if rectsWithColorInside(fills, notifRect, colSurface1) == 0 {
		t.Fatalf("no square colSurface1 fill drawn inside notif rect %v", notifRect)
	}
}
