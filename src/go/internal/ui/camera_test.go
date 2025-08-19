package ui

import "testing"

func TestCameraSnap(t *testing.T) {
	cam := &Camera{Scale: 1, OffsetX: 12.7, OffsetY: -3.4}
	cam.Snap()
	if cam.OffsetX != 13 || cam.OffsetY != -3 {
		t.Fatalf("rounded offsets=%f,%f want 13,-3", cam.OffsetX, cam.OffsetY)
	}
	cam.OffsetX = 2e6
	cam.OffsetY = -2e6
	cam.Snap()
	if cam.OffsetX != 1e6 || cam.OffsetY != -1e6 {
		t.Fatalf("clamped offsets=%f,%f", cam.OffsetX, cam.OffsetY)
	}
}
