//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func collectPillRects(h *SplitterHandle, y int) []image.Rectangle {
	dst := ebiten.NewImage(800, 600)
	var pills []image.Rectangle
	restore := interceptDrawRect(func(_ *ebiten.Image, r image.Rectangle, _ color.Color, filled bool) {
		if filled && r.Dx() == SplitterHandleLen() && r.Dy() == SplitterHandleThick() {
			pills = append(pills, r)
		}
	})
	defer restore()
	h.DrawHorizontalDivider(dst, 0, 800, y)
	return pills
}

// A pill hanging above the line detaches from the horizon and reads as a stray chip in the grid pane.
func TestSplitterPillStraddlesDividerLine(t *testing.T) {
	const y = 300
	h := &SplitterHandle{}
	pills := collectPillRects(h, y)
	if len(pills) == 0 {
		t.Fatal("no pill body rect drawn")
	}
	for _, r := range pills {
		if cy := (r.Min.Y + r.Max.Y) / 2; cy != y {
			t.Fatalf("pill center %d, want centered on line y=%d (rect %v)", cy, y, r)
		}
		if !(r.Min.Y < y && r.Max.Y > y) {
			t.Fatalf("pill %v does not straddle divider line y=%d", r, y)
		}
	}
}

// Through the real drawDivider: the shared handle alone would pass while NewSplitter opted into a different anchor.
func TestGridDrumSplitterPillCentersOnDividerLine(t *testing.T) {
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720) // desktop width → non-mobile divider path

	g.split.horizontal = true
	g.split.Y = 360

	var pills []image.Rectangle
	restore := interceptDrawRect(func(_ *ebiten.Image, r image.Rectangle, _ color.Color, filled bool) {
		if filled && r.Dx() == SplitterHandleLen() && r.Dy() == SplitterHandleThick() {
			pills = append(pills, r)
		}
	})
	g.drawDivider(ebiten.NewImage(1280, 720))
	restore()

	if len(pills) == 0 {
		t.Fatal("no pill body rect drawn by (*Game).drawDivider")
	}
	for _, r := range pills {
		if cy := (r.Min.Y + r.Max.Y) / 2; cy != g.split.Y {
			t.Fatalf("grid↔drum pill center %d, want centered on divider line y=%d (rect %v)",
				cy, g.split.Y, r)
		}
	}
}

// One pill geometry for both dividers, so a per-divider anchor cannot come back.
func TestBothDividerPillsShareGeometry(t *testing.T) {
	const y = 300
	main := collectPillRects(&NewSplitter(720).handle, y)        // grid↔drum
	eq := collectPillRects(&newRowEQDividerLayer(nil).handle, y) // row↔EQ
	if len(main) == 0 || len(eq) == 0 {
		t.Fatalf("missing pill rects: main=%d eq=%d", len(main), len(eq))
	}
	if main[0] != eq[0] {
		t.Fatalf("grid↔drum pill %v != row↔EQ pill %v — dividers drifted apart", main[0], eq[0])
	}
}
