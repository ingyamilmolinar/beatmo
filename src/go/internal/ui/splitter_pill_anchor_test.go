//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// collectPillRects draws a horizontal divider at y and returns every filled
// rect whose dimensions match the pill body (SplitterHandleLen ×
// SplitterHandleThick).
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

// Every SplitterHandle centres its pill ON the divider line — the pill must
// straddle the accent row, not hang above it. The main grid↔drum splitter used
// to anchor its pill above the line (PillAnchorAbove), which detached it from
// the horizon and read as a stray chip floating in the grid pane while the
// row↔EQ divider's pill sat correctly on its line.
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

// Functional counterpart: the REAL grid↔drum splitter, drawn through
// (*Game).drawDivider, must put its pill on the line exactly like the row↔EQ
// divider does. Asserting on the shared handle alone would not have caught the
// bug — NewSplitter opted its handle into the above-line anchor.
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

// Both production dividers must place their pill identically — one pill
// geometry, two dividers. Pins the two against each other so a future
// per-divider anchor cannot reintroduce the split.
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
