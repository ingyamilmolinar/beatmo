package ui

import "testing"

// A pan gesture (press-drag-release) emits exactly one pan event with the
// cumulative delta; a release with no movement emits nothing.
func TestCameraGestureEmitsOncePerPan(t *testing.T) {
	var pans []struct{ dx, dy float64 }
	g := &cameraGesture{emitPan: func(dx, dy float64) {
		pans = append(pans, struct{ dx, dy float64 }{dx, dy})
	}, emitZoom: func(float64) {}}

	g.observe(false, 0, 0, 1) // idle
	g.observe(true, 5, 0, 1)  // pan starts (offset jumps)
	g.observe(true, 12, 3, 1) // pan continues
	g.observe(false, 12, 3, 1) // release → one emit, dx=12 dy=3

	if len(pans) != 1 {
		t.Fatalf("want 1 pan emit, got %d: %+v", len(pans), pans)
	}
	if pans[0].dx != 12 || pans[0].dy != 3 {
		t.Fatalf("want dx=12 dy=3, got %+v", pans[0])
	}
}

func TestCameraGestureEmitsZoomOnScaleChange(t *testing.T) {
	var zooms []float64
	g := &cameraGesture{emitPan: func(float64, float64) {}, emitZoom: func(f float64) {
		zooms = append(zooms, f)
	}}
	g.observe(false, 0, 0, 1.0)
	g.observe(false, 0, 0, 1.1) // wheel zoom tick
	if len(zooms) != 1 {
		t.Fatalf("want 1 zoom emit, got %d", len(zooms))
	}
}
