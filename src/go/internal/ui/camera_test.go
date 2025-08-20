package ui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

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

func TestCameraZoomAnchorsCursor(t *testing.T) {
	cam := &Camera{Scale: 2, OffsetX: 10, OffsetY: 20}
	cursorX, cursorY := 100, 50
	restore := SetInputForTest(
		func() (int, int) { return cursorX, cursorY },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 1 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	wx := (float64(cursorX) - cam.OffsetX) / cam.Scale
	wy := (float64(cursorY) - cam.OffsetY) / cam.Scale
	cam.HandleMouse(true)
	sx, sy := cam.ScreenPos(wx, wy)
	if math.Abs(sx-float64(cursorX)) > 0.5 || math.Abs(sy-float64(cursorY)) > 0.5 {
		t.Fatalf("cursor moved after zoom: got (%f,%f) want (%d,%d)", sx, sy, cursorX, cursorY)
	}
	expected := 2 * math.Pow(1.05, 1)
	if math.Abs(cam.Scale-expected) > 1e-9 {
		t.Fatalf("scale=%f want %f", cam.Scale, expected)
	}
}
