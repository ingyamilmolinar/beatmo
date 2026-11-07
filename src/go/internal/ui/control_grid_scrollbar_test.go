//go:build test

package ui

import (
	"testing"
)

// A wide, grabbable gutter, not the 6 px default: on desktop the wheel over a control edits it, so the scrollbar is
// how rows scroll.
func TestControlGridScrollbarIsWideEnoughToTarget(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Skip("FM section does not overflow at this layout")
	}
	if w := grid.Scroll().Style.Width; w < 14 {
		t.Fatalf("control-grid scrollbar too thin to target: width=%d, want >= 14", w)
	}
	if bar := grid.Scroll().BarRect(); bar.Dx() < 14 {
		t.Fatalf("control-grid scrollbar BarRect too thin: width=%d, want >= 14", bar.Dx())
	}
}

// Exactly one row per notch over the scrollbar.
func TestControlGridWheelOverScrollbarAdvancesOneTick(t *testing.T) {
	g := modularSynthGameRealistic(t)
	grid := g.drum.SynthSectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Skip("FM section does not overflow at this layout")
	}
	bar := grid.Scroll().BarRect()
	bx, by := bar.Min.X+bar.Dx()/2, bar.Min.Y+bar.Dy()/2
	before := grid.Scroll().VS.First

	// Wheel DOWN (negative) over the bar advances First by exactly one row.
	injectWheelAt(t, g.drum, bx, by, 0, -1)
	g.drum.updateForTest()

	if got := grid.Scroll().VS.First; got != before+1 {
		t.Fatalf("wheel over the scrollbar did not advance one tick: First %d -> %d (want %d)", before, got, before+1)
	}
}
