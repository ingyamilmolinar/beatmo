//go:build test

package ui

// Visual styling and animation never diverge between mobile and desktop; layout and density may.

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

type roundedOp struct {
	rect       image.Rectangle
	r, g, b, a uint32
	radius     int
	filled     bool
}

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

func withProfile(t *testing.T, mobile bool, fn func()) {
	t.Helper()
	prev := forceSmallScreenForTest
	forceSmallScreenForTest = mobile
	UpdateProfile()
	t.Cleanup(func() { forceSmallScreenForTest = prev; UpdateProfile() })
	fn()
}

// The top-edge depth highlight renders on both platforms.
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

// Same fill, border and radius on both platforms.
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

// The recording-state ring must not diverge by platform.
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
