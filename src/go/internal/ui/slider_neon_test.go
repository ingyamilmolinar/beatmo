//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func captureRoundedForNeon(t *testing.T, fn func()) []roundedDraw {
	t.Helper()
	var calls []roundedDraw
	orig := drawRoundedRect
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		calls = append(calls, roundedDraw{Rect: r, Color: c, Radius: radius, Filled: filled})
		orig(dst, r, c, radius, filled)
	}
	t.Cleanup(func() { drawRoundedRect = orig })
	fn()
	return calls
}

func countFilled(calls []roundedDraw) int {
	n := 0
	for _, c := range calls {
		if c.Filled {
			n++
		}
	}
	return n
}

func TestSliderRail_FullValueDrawsRailAndFill(t *testing.T) {
	dst := ebiten.NewImage(100, 20)
	rail := image.Rect(10, 8, 90, 12)
	calls := captureRoundedForNeon(t, func() { drawSliderRail(dst, rail, 1.0, true, genColorPrimary) })
	if countFilled(calls) < 2 {
		t.Fatalf("expected >=2 filled rounded rects (rail+fill), got %d (%+v)", countFilled(calls), calls)
	}
}

func TestSliderRail_ZeroValueDrawsRailOnly(t *testing.T) {
	dst := ebiten.NewImage(100, 20)
	rail := image.Rect(10, 8, 90, 12)
	calls := captureRoundedForNeon(t, func() { drawSliderRail(dst, rail, 0.0, true, genColorPrimary) })
	if countFilled(calls) != 1 {
		t.Fatalf("zero value should draw rail only (1 filled), got %d", countFilled(calls))
	}
}

func TestSliderRail_HorizontalFillWidthTracksValue(t *testing.T) {
	dst := ebiten.NewImage(100, 20)
	rail := image.Rect(0, 8, 80, 12)
	var fillW int
	orig := drawRoundedRect
	drawRoundedRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		if filled && r.Dx() > 0 && r.Dx() < rail.Dx() {
			fillW = r.Dx()
		}
		orig(d, r, c, radius, filled)
	}
	t.Cleanup(func() { drawRoundedRect = orig })
	drawSliderRail(dst, rail, 0.5, true, genColorPrimary)
	if fillW != 40 {
		t.Fatalf("half value should fill exactly 40px of 80px rail, got %d", fillW)
	}
}

func TestSliderThumb_DrawsCoreCircle(t *testing.T) {
	dst := ebiten.NewImage(60, 60)
	calls := captureRoundedForNeon(t, func() { drawSliderThumb(dst, image.Pt(30, 30), 24, false, false) })
	if countFilled(calls) < 1 {
		t.Fatal("thumb should draw at least one filled rounded rect (the core)")
	}
	foundCore := false
	for _, c := range calls {
		if c.Filled && c.Radius == 12 {
			foundCore = true
		}
	}
	if !foundCore {
		t.Fatalf("expected a filled core with radius 12, got %+v", calls)
	}
}

func TestSliderThumb_SculptedShadowNoBloom(t *testing.T) {
	dst := ebiten.NewImage(60, 60)
	center := image.Pt(30, 30)
	dia := 24
	rad := dia / 2
	r := image.Rect(center.X-rad, center.Y-rad, center.X+rad, center.Y+rad)
	calls := captureRoundedForNeon(t, func() { drawSliderThumb(dst, center, dia, false, false) })

	// No bloom: nothing extends horizontally beyond the thumb rect.
	for _, c := range calls {
		if c.Rect.Min.X < r.Min.X || c.Rect.Max.X > r.Max.X {
			t.Fatalf("bloom/overspill: rounded rect %+v exceeds thumb x-bounds %v", c.Rect, r)
		}
	}
	// Contact shadow: a filled rect that extends one px below the thumb body.
	foundShadow := false
	for _, c := range calls {
		if c.Filled && c.Rect.Max.Y == r.Max.Y+1 {
			foundShadow = true
		}
	}
	if !foundShadow {
		t.Fatalf("expected a contact-shadow filled rect 1px below the thumb, got %+v", calls)
	}
}
