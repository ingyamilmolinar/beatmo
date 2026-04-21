//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestNarrowCellActiveFillCoversFullWidth(t *testing.T) {
	assertDefaultParityState(t)
	style := DrumCellStyle{
		On:        colStep,
		Off:       colStepOff,
		Highlight: colHighlight,
		Border:    colStepBorder,
	}
	r := image.Rect(10, 0, 14, 20) // 4px wide
	dst := ebiten.NewImage(100, 100)
	onCol := color.RGBA{200, 100, 50, 255}

	var rec drawCallRecorder
	rec.record(t, func() {
		style.Draw(dst, r, true, false, onCol)
	})

	// Fill should cover full rect width.
	var hasFill bool
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && c.Filled && c.Rect == r {
			hasFill = true
		}
	}
	if !hasFill {
		t.Fatal("expected filled rect covering full 4px cell width")
	}

	// No stroked (non-filled) border covering full rect.
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && !c.Filled && c.Rect == r {
			t.Fatal("narrow active cell should NOT have full stroked border")
		}
	}

	// Should have right-edge separator (1px wide, filled).
	rightEdge := image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y)
	var hasEdge bool
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && c.Filled && c.Rect == rightEdge {
			hasEdge = true
		}
	}
	if !hasEdge {
		t.Fatal("narrow active cell should have right-edge separator")
	}
}

func TestNarrowCellInactiveNoBorder(t *testing.T) {
	assertDefaultParityState(t)
	style := DrumCellStyle{
		On:        colStep,
		Off:       colStepOff,
		Highlight: colHighlight,
		Border:    colStepBorder,
	}
	r := image.Rect(0, 0, 4, 20) // 4px wide, inactive
	dst := ebiten.NewImage(100, 100)

	var rec drawCallRecorder
	rec.record(t, func() {
		style.Draw(dst, r, false, false, nil)
	})

	// Should have exactly 1 call: the fill.
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 drawRect call for narrow inactive cell, got %d", len(rec.calls))
	}
	if !rec.calls[0].Filled {
		t.Fatal("the single call should be a filled rect")
	}
}

func TestNormalWidthCellBorderPresent(t *testing.T) {
	assertDefaultParityState(t)
	style := DrumCellStyle{
		On:        colStep,
		Off:       colStepOff,
		Highlight: colHighlight,
		Border:    colStepBorder,
	}
	r := image.Rect(0, 0, 20, 20) // 20px wide — normal
	dst := ebiten.NewImage(100, 100)

	var rec drawCallRecorder
	rec.record(t, func() {
		style.Draw(dst, r, true, false, nil)
	})

	// Should have stroked border.
	var hasBorder bool
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && !c.Filled && c.Rect == r {
			hasBorder = true
		}
	}
	if !hasBorder {
		t.Fatal("normal-width active cell must have stroked border")
	}

	// Should have top strip (1px high, inset by 1 on each side).
	topStrip := image.Rect(r.Min.X+1, r.Min.Y, r.Max.X-1, r.Min.Y+1)
	var hasTopStrip bool
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && c.Filled && c.Rect == topStrip {
			hasTopStrip = true
		}
	}
	if !hasTopStrip {
		t.Fatal("normal-width active cell must have top strip highlight")
	}
}

func TestNarrowCellThresholdBoundary(t *testing.T) {
	assertDefaultParityState(t)
	style := DrumCellStyle{
		On:        colStep,
		Off:       colStepOff,
		Highlight: colHighlight,
		Border:    colStepBorder,
	}
	dst := ebiten.NewImage(100, 100)

	// 6px = narrow (at threshold) → no full border.
	r6 := image.Rect(0, 0, 6, 20)
	var rec drawCallRecorder
	rec.record(t, func() {
		style.Draw(dst, r6, true, false, nil)
	})
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && !c.Filled && c.Rect == r6 {
			t.Fatal("6px cell (at threshold) should NOT have full stroked border")
		}
	}

	// 7px = normal (above threshold) → has full border.
	r7 := image.Rect(0, 0, 7, 20)
	rec.record(t, func() {
		style.Draw(dst, r7, true, false, nil)
	})
	var hasBorder bool
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && !c.Filled && c.Rect == r7 {
			hasBorder = true
		}
	}
	if !hasBorder {
		t.Fatal("7px cell (above threshold) must have stroked border")
	}
}

func TestNarrowCellHighlighted(t *testing.T) {
	assertDefaultParityState(t)
	style := DrumCellStyle{
		On:        colStep,
		Off:       colStepOff,
		Highlight: colHighlight,
		Border:    colStepBorder,
	}
	r := image.Rect(5, 0, 8, 20) // 3px wide, highlighted
	dst := ebiten.NewImage(100, 100)
	onCol := color.RGBA{255, 0, 128, 255}

	var rec drawCallRecorder
	rec.record(t, func() {
		style.Draw(dst, r, false, true, onCol)
	})

	// Fill should use colHighlight (white flash), not instrument color.
	hlExpected := color.RGBAModel.Convert(colHighlight).(color.RGBA)
	var fillCol color.RGBA
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && c.Filled && c.Rect == r {
			fillCol = c.Color
		}
	}
	if fillCol != hlExpected {
		t.Fatalf("highlighted narrow cell fill = %v, want colHighlight %v", fillCol, hlExpected)
	}

	// Should have right-edge separator, no full border, no top strip.
	rightEdge := image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y)
	var hasEdge bool
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && !c.Filled && c.Rect == r {
			t.Fatal("narrow highlighted cell should NOT have full stroked border")
		}
		if c.Kind == drawCallRect && c.Filled && c.Rect == rightEdge {
			hasEdge = true
		}
	}
	if !hasEdge {
		t.Fatal("narrow highlighted cell should have right-edge separator")
	}
}

func TestNarrowCellMinimumWidth(t *testing.T) {
	assertDefaultParityState(t)
	style := DrumCellStyle{
		On:        colStep,
		Off:       colStepOff,
		Highlight: colHighlight,
		Border:    colStepBorder,
	}
	r := image.Rect(0, 0, 2, 20) // 2px — minimum width
	dst := ebiten.NewImage(100, 100)

	var rec drawCallRecorder
	rec.record(t, func() {
		style.Draw(dst, r, true, false, nil)
	})

	// Fill + right-edge separator = 2 calls.
	if len(rec.calls) != 2 {
		t.Fatalf("2px active cell: expected 2 drawRect calls, got %d", len(rec.calls))
	}

	// Right edge is 1px wide.
	rightEdge := image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y)
	var hasEdge bool
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && c.Filled && c.Rect == rightEdge {
			hasEdge = true
		}
	}
	if !hasEdge {
		t.Fatal("2px active cell must have 1px right-edge separator")
	}
}
