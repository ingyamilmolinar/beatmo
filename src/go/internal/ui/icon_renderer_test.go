package ui

import (
	"image"
	"math"
	"testing"
)

// 1.75 logical units on a 24-unit grid.
func TestIconStrokeWidthAt24px(t *testing.T) {
	got := iconStrokeWidth(image.Rect(0, 0, 24, 24))
	want := float32(1.75)
	if math.Abs(float64(got-want)) > 0.01 {
		t.Errorf("iconStrokeWidth(24x24) = %.3f; want ~%.3f", got, want)
	}
}

func TestIconStrokeWidthAt48px(t *testing.T) {
	got := iconStrokeWidth(image.Rect(0, 0, 48, 48))
	want := float32(3.5)
	if math.Abs(float64(got-want)) > 0.01 {
		t.Errorf("iconStrokeWidth(48x48) = %.3f; want %.3f", got, want)
	}
}

// The proportional weight at 8x8 is 0.583; the floor keeps it visible.
func TestIconStrokeWidthMinFloor(t *testing.T) {
	got := iconStrokeWidth(image.Rect(0, 0, 8, 8))
	if got < 1.0 {
		t.Errorf("iconStrokeWidth(8x8) = %.3f; want ≥ 1.0 (sub-pixel floor)", got)
	}
}

// Stroke comes from the shorter axis so it fits within it.
func TestIconStrokeWidthNonSquare(t *testing.T) {
	got := iconStrokeWidth(image.Rect(0, 0, 24, 16))
	want := float32(1.75) * 16 / 24 // 1.166...
	// Tolerance accounts for the 1.0 floor not kicking in (1.166 > 1).
	if math.Abs(float64(got-want)) > 0.01 {
		t.Errorf("iconStrokeWidth(24x16) = %.3f; want %.3f (uses minor axis)", got, want)
	}
}

func TestIconCanvasMappingCenter(t *testing.T) {
	c := newIconCanvas(image.Rect(10, 20, 34, 44)) // 24x24, top-left (10,20)
	gotX, gotY := c.px(12, 12)
	wantX, wantY := float32(22), float32(32) // (10+24/2, 20+24/2)
	if math.Abs(float64(gotX-wantX)) > 0.01 || math.Abs(float64(gotY-wantY)) > 0.01 {
		t.Errorf("c.px(12,12) = (%.2f, %.2f); want (%.2f, %.2f)", gotX, gotY, wantX, wantY)
	}
}

func TestIconCanvasMappingScale(t *testing.T) {
	c := newIconCanvas(image.Rect(0, 0, 48, 48))
	x, y := c.px(0, 0)
	if math.Abs(float64(x-0)) > 0.01 || math.Abs(float64(y-0)) > 0.01 {
		t.Errorf("c.px(0,0) = (%.2f, %.2f); want (0,0)", x, y)
	}
	x, y = c.px(1, 1)
	if math.Abs(float64(x-2)) > 0.01 || math.Abs(float64(y-2)) > 0.01 {
		t.Errorf("c.px(1,1) at 48x48 = (%.2f, %.2f); want (2,2)", x, y)
	}
}

func TestIconCanvasNonSquareCentered(t *testing.T) {
	c := newIconCanvas(image.Rect(0, 0, 32, 24))
	// Logical (0, 0) should land at pixel (4, 0) — 4-px left margin.
	x, y := c.px(0, 0)
	if math.Abs(float64(x-4)) > 0.01 || math.Abs(float64(y-0)) > 0.01 {
		t.Errorf("c.px(0,0) for 32x24 = (%.2f, %.2f); want (4, 0)", x, y)
	}
}
