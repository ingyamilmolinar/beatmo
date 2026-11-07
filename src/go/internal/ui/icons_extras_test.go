package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Dispatch only: the stub backend does not reflect vector draws in dst.At.
func TestIconStarDispatch(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	col := color.NRGBA{255, 255, 255, 255}
	for _, id := range []IconID{IconStar, IconStarFilled, IconChevronLeft} {
		if !drawIconByID(dst, id, r, col) {
			t.Errorf("drawIconByID returned false for %q; switch case missing", id)
		}
	}
}

// Callers fall back to a text label when drawIconByID returns false.
func TestIconUnknownReturnsFalse(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	col := color.NRGBA{255, 255, 255, 255}
	if drawIconByID(dst, IconID("not-a-real-icon"), r, col) {
		t.Errorf("drawIconByID returned true for unknown id; switch default broken")
	}
}

// Ten vertices, alternating outer/inner; anything else means the path table was damaged.
func TestStarPathPointCount(t *testing.T) {
	if got := len(starPathPoints); got != 10 {
		t.Fatalf("starPathPoints len=%d, want 10 (5 outer + 5 inner)", got)
	}
}

func TestIconNodeActionDispatch(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	col := color.NRGBA{255, 255, 255, 255}
	for _, id := range []IconID{IconMove, IconCopy, IconConnect} {
		if !drawIconByID(dst, id, r, col) {
			t.Errorf("drawIconByID returned false for %q; switch case or IconID missing", id)
		}
	}
}

// Dispatch only: the stub backend does not reflect vector draws in dst.At.
func TestIconFolderDispatch(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	col := color.NRGBA{255, 255, 255, 255}
	if !drawIconByID(dst, IconFolder, r, col) {
		t.Fatal("drawIconByID returned false for IconFolder; switch case or IconID missing")
	}
}
