package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestIconStarDispatch verifies the new IconStar / IconStarFilled /
// IconChevronLeft entries are wired through drawIconByID. The body
// just exercises the switch — pixel-level rendering is covered by
// the broader icon_renderer tests.
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

// TestIconUnknownReturnsFalse pins the contract that drawIconByID
// returns false for unmapped IconID values. Callers rely on this to
// fall back to a text label.
func TestIconUnknownReturnsFalse(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	col := color.NRGBA{255, 255, 255, 255}
	if drawIconByID(dst, IconID("not-a-real-icon"), r, col) {
		t.Errorf("drawIconByID returned true for unknown id; switch default broken")
	}
}

// TestStarPathPointCount sanity-checks that the star polygon has the
// expected ten vertices (alternating outer/inner). A regression here
// usually means a hand edit damaged the path table.
func TestStarPathPointCount(t *testing.T) {
	if got := len(starPathPoints); got != 10 {
		t.Fatalf("starPathPoints len=%d, want 10 (5 outer + 5 inner)", got)
	}
}

// TestIconNodeActionDispatch verifies the node quick-action icons are wired
// through drawIconByID (used by the long-press popup keycaps).
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

// TestIconFolderDispatch verifies the folder glyph (grid-pane file button) is
// wired through drawIconByID. Pixel-level rendering is covered by the broader
// icon_renderer tests; the stub backend does not reflect vector draws in
// dst.At, so this only exercises the dispatch switch (matching the other icon
// dispatch tests in this file).
func TestIconFolderDispatch(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	col := color.NRGBA{255, 255, 255, 255}
	if !drawIconByID(dst, IconFolder, r, col) {
		t.Fatal("drawIconByID returned false for IconFolder; switch case or IconID missing")
	}
}
