//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Real Draw at the production viewport: rect math can pass while the paint lands wrong (a pill bleeding into the
// bar, an icon rendering nothing).

// dv.Draw paints in screen coordinates, so dst spans (0,0)..Bounds.Max.
func drawScreenForBounds(dv *DrumView) *ebiten.Image {
	w := dv.Bounds.Max.X
	h := dv.Bounds.Max.Y
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return ebiten.NewImage(w, h)
}

func TestStartupChrome_BeatCounterPillPaintsInsideRect_Desktop(t *testing.T) {
	assertDefaultParityState(t)
	dv := productionDrumView(t, 1280, 720)
	dst := drawScreenForBounds(dv)

	rec := &drawCallRecorder{}
	rec.record(t, func() {
		dv.Draw(dst, nil, 0, nil, 0)
	})

	bc := dv.beatCounterRect
	if bc.Empty() {
		t.Fatal("beatCounterRect is empty")
	}
	// Matched by signature (the shared fill, overlapping and no larger than the band) so larger zone fills do not
	// pose as the cap.
	pillCalls := beatCounterCapCalls(rec, bc)
	if len(pillCalls) == 0 {
		t.Fatalf("no beat counter keycap cap face found in beatCounterRect=%v (%d total calls captured)",
			bc, len(rec.calls))
	}
	// Never below the band; a ≤1 px lift above is the intentional active raise.
	for _, c := range pillCalls {
		if beatCounterCapBleeds(c.Rect, bc) {
			t.Errorf("beat counter keycap call %v bleeds outside beatCounterRect=%v", c.Rect, bc)
		}
	}
}

// Only that paint lands in the rect, not zero area; contrast is a design question.
func TestStartupChrome_TrackButtonPaints_Desktop(t *testing.T) {
	assertDefaultParityState(t)
	dv := productionDrumView(t, 1280, 720)
	dst := drawScreenForBounds(dv)

	rec := &drawCallRecorder{}
	rec.record(t, func() {
		dv.Draw(dst, nil, 0, nil, 0)
	})

	tb := dv.trackBtn().Rect()
	if tb.Empty() {
		t.Fatal("trackBtn rect is empty")
	}
	for _, c := range rec.calls {
		if (c.Kind == drawCallButton || c.Kind == drawCallRoundedButton) && c.Rect == tb {
			return // success
		}
	}
	// Fall back: any drawRect/drawRoundedRect whose rect equals tb (legacy paths).
	for _, c := range rec.calls {
		if (c.Kind == drawCallRect || c.Kind == drawCallRoundedRect) && c.Filled && c.Rect == tb {
			return
		}
	}
	t.Fatalf("no body paint call landed at trackBtn rect=%v (%d total calls captured)",
		tb, len(rec.calls))
}

// Guards against a bar sized but off-screen or clipped to zero.
func TestStartupChrome_TimelineBarPaintsCursor_Desktop(t *testing.T) {
	assertDefaultParityState(t)
	dv := productionDrumView(t, 1280, 720)
	dst := drawScreenForBounds(dv)

	rec := &drawCallRecorder{}
	rec.record(t, func() {
		dv.Draw(dst, nil, 0, nil, 0)
	})

	bar := dv.timelineRect
	if bar.Empty() {
		t.Fatal("timelineRect is empty")
	}
	for _, c := range rec.calls {
		if c.Kind != drawCallRect || !c.Filled {
			continue
		}
		if c.Color != colTimelineCursor {
			continue
		}
		if c.Rect.Overlaps(bar) {
			return // cursor wedge painted inside the bar — success
		}
	}
	t.Fatalf("no timeline cursor drawRect(colTimelineCursor) painted inside timelineRect=%v (%d total calls captured)",
		bar, len(rec.calls))
}

func TestStartupChrome_BeatCounterPillPaintsInsideRect_Mobile(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)
	dv := productionDrumView(t, 390, 844)
	dst := drawScreenForBounds(dv)

	rec := &drawCallRecorder{}
	rec.record(t, func() {
		dv.Draw(dst, nil, 0, nil, 0)
	})

	bc := dv.beatCounterRect
	if bc.Empty() {
		t.Fatal("beatCounterRect is empty on mobile")
	}
	pillCalls := beatCounterCapCalls(rec, bc)
	if len(pillCalls) == 0 {
		t.Fatalf("no beat counter keycap cap face found in beatCounterRect=%v (%d total calls captured)",
			bc, len(rec.calls))
	}
	for _, c := range pillCalls {
		if beatCounterCapBleeds(c.Rect, bc) {
			t.Errorf("mobile beat counter keycap call %v bleeds outside beatCounterRect=%v", c.Rect, bc)
		}
	}
}

// Past either side, below the bottom, or above the top by more than the active raise.
func beatCounterCapBleeds(cap, bc image.Rectangle) bool {
	if cap.Min.X < bc.Min.X || cap.Max.X > bc.Max.X {
		return true
	}
	if cap.Max.Y > bc.Max.Y {
		return true
	}
	if cap.Min.Y < bc.Min.Y-genGeomKeycapActiveRaise {
		return true
	}
	return false
}

// Fill-matched, overlapping the band and no larger than it.
func beatCounterCapCalls(rec *drawCallRecorder, bc image.Rectangle) []drawCall {
	restFill := color.RGBAModel.Convert(TransportMiscStyle.Fill).(color.RGBA)
	hoverFill := color.RGBAModel.Convert(adjustColor(TransportMiscStyle.Fill, 12)).(color.RGBA)
	var out []drawCall
	for _, c := range rec.calls {
		// Either draw path is accepted.
		switch {
		case c.Kind == drawCallRoundedButton:
		case c.Kind == drawCallRoundedRect && c.Filled:
		default:
			continue
		}
		if c.Color != restFill && c.Color != hoverFill {
			continue
		}
		if !c.Rect.Overlaps(bc) || c.Rect.Dx() > bc.Dx() || c.Rect.Dy() > bc.Dy() {
			continue
		}
		out = append(out, c)
	}
	return out
}

// (image package unused-import guard — keep image.Rectangle reference live)
var _ = image.Rectangle{}
