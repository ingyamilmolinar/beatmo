//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// These tests run the real DrumView.Draw at the production capture viewport
// and use drawCallRecorder to confirm the chrome rects (beat counter pill,
// track button body) actually receive paint calls. They are the pixel-
// evidence companion to drumview_startup_chrome_test.go: even when the
// rect math passes, paint-into-rect can be wrong (e.g. the pill sized
// taller than its rect bleeds into the timeline bar; the track button
// rect is fine but the icon sprite renders zero pixels).

// drawScreenForBounds returns a *ebiten.Image sized to fully contain the
// drum view's screen-space bounds. dv.Draw paints into screen coordinates,
// so the dst must include (0,0)..(dv.Bounds.Max.X, dv.Bounds.Max.Y).
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

// TestStartupChrome_BeatCounterPillPaintsInsideRect_Desktop asserts the
// drawRoundedRect call that paints the pill background lands fully inside
// dv.beatCounterRect. The current bug: pillH (22 px) exceeds infoH (20 px)
// so the pill bottom sits at beatCounterRect.Max.Y+1, intruding into the
// timeline bar.
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
	// Find the keycap cap face. The counter now renders as a matte keycap
	// (drawKeycapPanelButton → drawRoundedButton with the shared secondary
	// fill), NOT a flat drawRect. Match by signature — the cap face uses
	// TransportMiscStyle.Fill (rest or hover-brightened), overlaps the band, and
	// is no larger than it — so larger zone backgrounds don't masquerade as it.
	pillCalls := beatCounterCapCalls(rec, bc)
	if len(pillCalls) == 0 {
		t.Fatalf("no beat counter keycap cap face found in beatCounterRect=%v (%d total calls captured)",
			bc, len(rec.calls))
	}
	// The keycap must not bleed BELOW the band into the timeline bar (the
	// regression this guards). A ≤1px lift above the top is the keycap's
	// intentional active-raise and is allowed.
	for _, c := range pillCalls {
		if beatCounterCapBleeds(c.Rect, bc) {
			t.Errorf("beat counter keycap call %v bleeds outside beatCounterRect=%v", c.Rect, bc)
		}
	}
}

// TestStartupChrome_TrackButtonPaints_Desktop asserts the track button's
// body render call (drawButton via Render) lands at trackBtn.Rect().
// Today the rect is correct but the button uses a low-contrast surface
// style on a dark header; this test only asserts paint occurs inside
// the rect. Visual contrast is left to the design pass — what matters
// for the bug report is that the button isn't rendered at zero area.
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
	// The button body paints via drawButton (via Render) at exactly tb.
	// Any drawButton/drawRoundedButton call whose rect equals tb counts.
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

// TestStartupChrome_TimelineBarPaintsCursor_Desktop asserts the timeline
// bar paints the cursor wedge inside its bar rect. This guards against a
// bar that is sized but positioned off-screen or clipped to zero by a
// future tlZoneRect computation.
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

// TestStartupChrome_BeatCounterPillPaintsInsideRect_Mobile mobile counterpart.
// Locks in the invariant once we fix the pill clamp.
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
	// Matte keycap cap face (drawRoundedButton with the shared secondary fill),
	// no larger than the band (see desktop counterpart for rationale).
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

// beatCounterCapBleeds reports whether keycap cap rect `cap` escapes band `bc`
// in a way that matters: past either horizontal edge, below the bottom (into
// the timeline bar — the regression guarded), or more than the keycap's
// active-raise above the top. A ≤genGeomKeycapActiveRaise lift above the top is
// intentional keycap chrome and allowed.
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

// beatCounterCapCalls returns the recorded keycap cap-face draws that make up
// the beat-counter keycap inside band `bc`: drawRoundedButton calls painted
// with the shared secondary fill (rest or hover-brightened), overlapping the
// band and no larger than it. Shared by the desktop + mobile startup-chrome
// pill-paint assertions.
func beatCounterCapCalls(rec *drawCallRecorder, bc image.Rectangle) []drawCall {
	restFill := color.RGBAModel.Convert(TransportMiscStyle.Fill).(color.RGBA)
	hoverFill := color.RGBAModel.Convert(adjustColor(TransportMiscStyle.Fill, 12)).(color.RGBA)
	var out []drawCall
	for _, c := range rec.calls {
		// Flat-tonal (2026-07-25): the counter body is a FILLED rounded rect
		// (drawFlatButtonBody), no longer a drawRoundedButton fill+border
		// composite. Accept both so this helper survives either path.
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
