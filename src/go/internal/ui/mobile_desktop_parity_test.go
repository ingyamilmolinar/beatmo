//go:build test

package ui

// mobile_desktop_parity_test.go — cross-platform visual-parity guards.
//
// These tests pin the rule that VISUAL STYLING and ANIMATION never diverge
// between mobile and desktop. (Layout arrangement and Density-driven sizing
// may still diverge — those axes are exercised elsewhere.) Each test forces a
// profile and asserts mobile renders the same treatment desktop does.

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// roundedOp records one drawRoundedRect call (rect + premultiplied color +
// radius + filled).
type roundedOp struct {
	rect           image.Rectangle
	r, g, b, a     uint32
	radius         int
	filled         bool
}

// captureRounded intercepts drawRoundedRect and records every call. Returns a
// snapshot getter + restore.
func captureRounded() (ops func() []roundedOp, restore func()) {
	var got []roundedOp
	orig := drawRoundedRect
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, cc color.Color, radius int, filled bool) {
		cr, cg, cb, ca := cc.RGBA()
		got = append(got, roundedOp{r, cr, cg, cb, ca, radius, filled})
		orig(dst, r, cc, radius, filled)
	}
	return func() []roundedOp { return got }, func() { drawRoundedRect = orig }
}

// withProfile runs fn with the mobile/desktop profile forced, restoring the
// previous profile afterward.
func withProfile(t *testing.T, mobile bool, fn func()) {
	t.Helper()
	prev := forceSmallScreenForTest
	forceSmallScreenForTest = mobile
	UpdateProfile()
	t.Cleanup(func() { forceSmallScreenForTest = prev; UpdateProfile() })
	fn()
}

// TestTopEdgeHighlightParity pins that the subtle top-edge depth highlight on
// buttons renders on BOTH platforms (was desktop-only under the retired
// "mobile chrome stays flat" rule).
func TestTopEdgeHighlightParity(t *testing.T) {
	var desktop, mobile bool
	withProfile(t, false, func() { desktop = Profile().DrawTopEdgeHighlight })
	withProfile(t, true, func() {
		if !Profile().IsMobile() {
			t.Skip("could not force mobile profile")
		}
		mobile = Profile().DrawTopEdgeHighlight
	})
	if !desktop {
		t.Fatalf("precondition: desktop DrawTopEdgeHighlight should be true, got false")
	}
	if mobile != desktop {
		t.Fatalf("DrawTopEdgeHighlight diverges by platform: desktop=%v mobile=%v (visual styling must not diverge)", desktop, mobile)
	}
}

// TestTransportGroupStyleParity pins that the transport cluster pill (play +
// stop + record container) renders with the SAME fill, border, and radius on
// both platforms — previously mobile used colSurface2 + colBorderMedium +
// RadiusMD+2 while desktop used colTransportGroupBG + colTransportGroupBorder +
// RadiusMD.
func TestTransportGroupStyleParity(t *testing.T) {
	groupRect := image.Rect(10, 10, 120, 44)
	capture := func(mobile bool) []roundedOp {
		var ops []roundedOp
		withProfile(t, mobile, func() {
			if mobile && !Profile().IsMobile() {
				t.Skip("could not force mobile profile")
			}
			z := &TransportZone{transportGroupRect: groupRect}
			get, restore := captureRounded()
			defer restore()
			img := ebiten.NewImage(200, 80)
			z.drawTransportGroupOffset(img, 0, 0)
			ops = get()
		})
		return ops
	}
	desktop := capture(false)
	mobile := capture(true)
	if len(desktop) == 0 {
		t.Fatal("desktop transport group drew nothing")
	}
	if len(mobile) != len(desktop) {
		t.Fatalf("transport group op count diverges: desktop=%d mobile=%d", len(desktop), len(mobile))
	}
	for i := range desktop {
		if desktop[i] != mobile[i] {
			t.Errorf("transport group op %d diverges:\n desktop=%+v\n mobile =%+v", i, desktop[i], mobile[i])
		}
	}
}

// TestRecordArmedRingParity pins that the destructive recording-armed ring
// around the record button renders on BOTH platforms (was mobile-only). The
// recording-state highlight must not diverge by platform.
func TestRecordArmedRingParity(t *testing.T) {
	want := genColorDestructive
	dr, dg, db, da := want.RGBA()
	ringDrawn := func(mobile bool) bool {
		var found bool
		withProfile(t, mobile, func() {
			if mobile && !Profile().IsMobile() {
				t.Skip("could not force mobile profile")
			}
			z := &TransportZone{}
			z.recordBtn = NewButton("", nil, func() {})
			z.recordBtn.SetRect(image.Rect(40, 10, 70, 40))
			z.SetRecording(true)
			get, restore := captureRounded()
			defer restore()
			img := ebiten.NewImage(120, 60)
			z.drawRecordArmedRingOffset(img, 0, 0)
			for _, op := range get() {
				if !op.filled && op.r == dr && op.g == dg && op.b == db && op.a == da {
					found = true
				}
			}
		})
		return found
	}
	if !ringDrawn(true) {
		t.Fatal("recording-armed ring not drawn on mobile (precondition)")
	}
	if !ringDrawn(false) {
		t.Fatal("recording-armed ring not drawn on desktop — recording highlight diverges by platform")
	}
}
