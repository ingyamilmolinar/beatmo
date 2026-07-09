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

// The main grid↔drum splitter's pill must not protrude below the divider
// line into the transport bar — pre-fix the centered pill occluded the
// "Beat N · M:SS" readout at window widths where the clock chip reached
// mid-screen (A6 in the 2026-07-04 screenshot critique). PillAnchorAbove
// hangs the pill into the spacious grid pane instead; hit/hover rects stay
// centered (extra forgiveness below, no drag-affordance change).
func TestSplitterPillAnchorAboveKeepsPillAboveLine(t *testing.T) {
	const y = 300
	h := &SplitterHandle{PillAnchorAbove: true}
	pills := collectPillRects(h, y)
	if len(pills) == 0 {
		t.Fatal("no pill body rect drawn")
	}
	for _, r := range pills {
		if r.Max.Y > y {
			t.Fatalf("pill body %v protrudes below divider line y=%d", r, y)
		}
	}
}

// Default handles (the row↔EQ divider) keep the legacy centered pill.
func TestSplitterPillDefaultStaysCentered(t *testing.T) {
	const y = 300
	h := &SplitterHandle{}
	pills := collectPillRects(h, y)
	if len(pills) == 0 {
		t.Fatal("no pill body rect drawn")
	}
	for _, r := range pills {
		if cy := (r.Min.Y + r.Max.Y) / 2; cy != y {
			t.Fatalf("default pill center %d, want centered on line y=%d", cy, y)
		}
	}
}
