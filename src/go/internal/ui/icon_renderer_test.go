package ui

import (
	"image"
	"math"
	"testing"
)

// TestIconStrokeWidthAt24px asserts the proportional stroke calc returns
// the spec-defined logical-unit weight at the canonical 24-px size.
// 1.75 logical units * (24 px / 24 grid) = 1.75 px.
func TestIconStrokeWidthAt24px(t *testing.T) {
	got := iconStrokeWidth(image.Rect(0, 0, 24, 24))
	want := float32(1.75)
	if math.Abs(float64(got-want)) > 0.01 {
		t.Errorf("iconStrokeWidth(24x24) = %.3f; want ~%.3f", got, want)
	}
}

// TestIconStrokeWidthAt48px confirms stroke scales linearly with rect size:
// 1.75 * (48/24) = 3.50.
func TestIconStrokeWidthAt48px(t *testing.T) {
	got := iconStrokeWidth(image.Rect(0, 0, 48, 48))
	want := float32(3.5)
	if math.Abs(float64(got-want)) > 0.01 {
		t.Errorf("iconStrokeWidth(48x48) = %.3f; want %.3f", got, want)
	}
}

// TestIconStrokeWidthMinFloor pins the 1-px floor: at 8x8 the proportional
// calc gives 0.583, but stroke must clamp to ≥1 to remain visible.
func TestIconStrokeWidthMinFloor(t *testing.T) {
	got := iconStrokeWidth(image.Rect(0, 0, 8, 8))
	if got < 1.0 {
		t.Errorf("iconStrokeWidth(8x8) = %.3f; want ≥ 1.0 (sub-pixel floor)", got)
	}
}

// TestIconStrokeWidthNonSquare uses the smaller dimension. A 24x16 rect
// must compute stroke from 16 (not 24) so the stroke fits within the
// shorter axis.
func TestIconStrokeWidthNonSquare(t *testing.T) {
	got := iconStrokeWidth(image.Rect(0, 0, 24, 16))
	want := float32(1.75) * 16 / 24 // 1.166...
	// Tolerance accounts for the 1.0 floor not kicking in (1.166 > 1).
	if math.Abs(float64(got-want)) > 0.01 {
		t.Errorf("iconStrokeWidth(24x16) = %.3f; want %.3f (uses minor axis)", got, want)
	}
}

// TestIconCanvasMappingCenter confirms logical (12, 12) maps to the rect's
// pixel center for a 24-px square.
func TestIconCanvasMappingCenter(t *testing.T) {
	c := newIconCanvas(image.Rect(10, 20, 34, 44)) // 24x24, top-left (10,20)
	gotX, gotY := c.px(12, 12)
	wantX, wantY := float32(22), float32(32) // (10+24/2, 20+24/2)
	if math.Abs(float64(gotX-wantX)) > 0.01 || math.Abs(float64(gotY-wantY)) > 0.01 {
		t.Errorf("c.px(12,12) = (%.2f, %.2f); want (%.2f, %.2f)", gotX, gotY, wantX, wantY)
	}
}

// TestIconCanvasMappingScale: at 48x48 the scale should be 2 px / logical
// unit, and a logical (1,1) offset from origin should be 2 px from origin.
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

// TestIconCanvasNonSquareCentered: a 32x24 rect should center the 24x24
// canvas horizontally — 4-px margin on each side.
func TestIconCanvasNonSquareCentered(t *testing.T) {
	c := newIconCanvas(image.Rect(0, 0, 32, 24))
	// Logical (0, 0) should land at pixel (4, 0) — 4-px left margin.
	x, y := c.px(0, 0)
	if math.Abs(float64(x-4)) > 0.01 || math.Abs(float64(y-0)) > 0.01 {
		t.Errorf("c.px(0,0) for 32x24 = (%.2f, %.2f); want (4, 0)", x, y)
	}
}
