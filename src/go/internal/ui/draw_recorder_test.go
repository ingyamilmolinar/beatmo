//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// ── draw call recorder ──────────────────────────────────────────────────

type drawCallKind int

const (
	drawCallRect drawCallKind = iota
	drawCallRoundedRect
	drawCallButton
	drawCallRoundedButton
)

type drawCall struct {
	Seq    int
	Kind   drawCallKind
	Rect   image.Rectangle
	Color  color.RGBA
	Filled bool
	Radius int
}

type drawCallRecorder struct {
	calls []drawCall
	seq   int
}

// record intercepts drawRect, drawRoundedRect, drawButton, and drawRoundedButton
// during fn() and records all calls. Original functions are still invoked.
func (rec *drawCallRecorder) record(t *testing.T, fn func()) {
	t.Helper()
	rec.calls = nil
	rec.seq = 0

	origRect := drawRect
	origRounded := drawRoundedRect
	origButton := drawButton
	origRoundedBtn := drawRoundedButton

	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		rec.calls = append(rec.calls, drawCall{
			Seq:    rec.seq,
			Kind:   drawCallRect,
			Rect:   r,
			Color:  color.RGBAModel.Convert(c).(color.RGBA),
			Filled: filled,
		})
		rec.seq++
		origRect(dst, r, c, filled)
	}
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		rec.calls = append(rec.calls, drawCall{
			Seq:    rec.seq,
			Kind:   drawCallRoundedRect,
			Rect:   r,
			Color:  color.RGBAModel.Convert(c).(color.RGBA),
			Filled: filled,
			Radius: radius,
		})
		rec.seq++
		origRounded(dst, r, c, radius, filled)
	}
	drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed, topEdgeHighlight bool) {
		rec.calls = append(rec.calls, drawCall{
			Seq:   rec.seq,
			Kind:  drawCallButton,
			Rect:  r,
			Color: color.RGBAModel.Convert(fill).(color.RGBA),
		})
		rec.seq++
		origButton(dst, r, fill, border, pressed, topEdgeHighlight)
	}
	drawRoundedButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, radius int, pressed bool) {
		rec.calls = append(rec.calls, drawCall{
			Seq:    rec.seq,
			Kind:   drawCallRoundedButton,
			Rect:   r,
			Color:  color.RGBAModel.Convert(fill).(color.RGBA),
			Radius: radius,
		})
		rec.seq++
		origRoundedBtn(dst, r, fill, border, radius, pressed)
	}

	t.Cleanup(func() {
		drawRect = origRect
		drawRoundedRect = origRounded
		drawButton = origButton
		drawRoundedButton = origRoundedBtn
	})

	fn()
}

// inRegion returns all calls whose Rect overlaps r.
func (rec *drawCallRecorder) inRegion(r image.Rectangle) []drawCall {
	var out []drawCall
	for _, c := range rec.calls {
		if c.Rect.Overlaps(r) {
			out = append(out, c)
		}
	}
	return out
}

// hasRoundedButton returns true if any drawCallRoundedButton call matches r
// with radius >= minRadius.
func (rec *drawCallRecorder) hasRoundedButton(r image.Rectangle, minRadius int) bool {
	for _, c := range rec.calls {
		if c.Kind == drawCallRoundedButton && c.Rect == r && c.Radius >= minRadius {
			return true
		}
	}
	return false
}

// hasSquareButton returns true if any drawCallButton call matches r.
func (rec *drawCallRecorder) hasSquareButton(r image.Rectangle) bool {
	for _, c := range rec.calls {
		if c.Kind == drawCallButton && c.Rect == r {
			return true
		}
	}
	return false
}

// assertNoSquareButtons fails if any drawCallButton call overlaps r.
func (rec *drawCallRecorder) assertNoSquareButtons(t *testing.T, r image.Rectangle) {
	t.Helper()
	for _, c := range rec.calls {
		if c.Kind == drawCallButton && c.Rect.Overlaps(r) {
			t.Errorf("found square drawButton call at %v inside region %v", c.Rect, r)
		}
	}
}

// ── corner shape analysis ───────────────────────────────────────────────

// cornerFillWidths calls drawRoundedRect with the given params, intercepts the
// inner drawRect calls, and returns the fill width at each row in the top-left
// corner region (rows 0..radius-1). Width = how many pixels filled from the
// left edge inward.
func cornerFillWidths(t *testing.T, radius int) []int {
	t.Helper()
	r := image.Rect(0, 0, 4*radius, 4*radius)
	c := color.RGBA{255, 0, 0, 255}

	var rects []drawnRect
	origRect := drawRect
	drawRect = func(dst *ebiten.Image, dr image.Rectangle, dc color.Color, filled bool) {
		if filled {
			rects = append(rects, drawnRect{
				Rect:  dr,
				Color: color.RGBAModel.Convert(dc).(color.RGBA),
			})
		}
		origRect(dst, dr, dc, filled)
	}
	defer func() { drawRect = origRect }()

	img := ebiten.NewImage(r.Dx(), r.Dy())
	drawRoundedRect(img, r, c, radius, true)

	// Extract top-left corner fill widths. The corner rects have
	// y in [0, radius) and x starting at some inset from 0.
	widths := make([]int, radius)
	for _, dr := range rects {
		if dr.Color != c {
			continue
		}
		// Top-left corner: rect from (inset, row) to (radius, row+1)
		if dr.Rect.Min.Y >= 0 && dr.Rect.Min.Y < radius &&
			dr.Rect.Dy() == 1 &&
			dr.Rect.Max.X == radius {
			row := dr.Rect.Min.Y
			w := dr.Rect.Dx()
			widths[row] = w
		}
	}
	return widths
}

// strokeCornerInsets calls drawRoundedRect (stroked) and returns the inset of
// the stroke pixel at each row in the top-left corner (distance from left edge).
func strokeCornerInsets(t *testing.T, radius int) []int {
	t.Helper()
	r := image.Rect(0, 0, 4*radius, 4*radius)
	c := color.RGBA{0, 255, 0, 255}

	var rects []drawnRect
	origRect := drawRect
	drawRect = func(dst *ebiten.Image, dr image.Rectangle, dc color.Color, filled bool) {
		if filled {
			rects = append(rects, drawnRect{
				Rect:  dr,
				Color: color.RGBAModel.Convert(dc).(color.RGBA),
			})
		}
		origRect(dst, dr, dc, filled)
	}
	defer func() { drawRect = origRect }()

	img := ebiten.NewImage(r.Dx(), r.Dy())
	drawRoundedRect(img, r, c, radius, false)

	// Top-left corner stroke: 1x1 pixel rects at (inset, row).
	insets := make([]int, radius)
	for i := range insets {
		insets[i] = -1 // sentinel: not found
	}
	for _, dr := range rects {
		if dr.Color != c {
			continue
		}
		if dr.Rect.Min.Y >= 0 && dr.Rect.Min.Y < radius &&
			dr.Rect.Dx() == 1 && dr.Rect.Dy() == 1 &&
			dr.Rect.Min.X < radius {
			row := dr.Rect.Min.Y
			insets[row] = dr.Rect.Min.X
		}
	}
	return insets
}

// assertCornerConvex verifies that corner fill widths increase monotonically
// from tip (row 0) to base (row radius-1).
func assertCornerConvex(t *testing.T, widths []int) {
	t.Helper()
	for i := 1; i < len(widths); i++ {
		if widths[i] < widths[i-1] {
			t.Errorf("corner not convex: row %d width=%d < row %d width=%d (widths=%v)",
				i, widths[i], i-1, widths[i-1], widths)
			return
		}
	}
}
