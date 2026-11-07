package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The cells fill the timeline widget's whole width. The ribbon bar maps the whole song while the cells map the
// current window, so it was never their anchor; CellGridRect is.

// On desktop and mobile.
func TestCellGridRectFillsDedicatedWidth(t *testing.T) {
	assertDefaultParityState(t)

	cases := []struct {
		name   string
		w, h   int
		mobile bool
	}{
		{"desktop", 1280, 720, false},
		{"mobile", 390, 844, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forceSmallScreenForTest = tc.mobile
			t.Cleanup(func() { forceSmallScreenForTest = false })

			dv := newTestDrumView(t, tc.w, tc.h)
			dv.calcLayout()

			dedicated := dv.widgetRects[WidgetTimeline]
			if dedicated.Empty() {
				t.Fatalf("timeline widget rect empty; layout did not run")
			}
			cells := dv.CellGridRect()
			if cells.Min.X != dedicated.Min.X {
				t.Errorf("cell grid left edge = %d, want %d (dedicated space left edge); %d px black gap after the row controls",
					cells.Min.X, dedicated.Min.X, cells.Min.X-dedicated.Min.X)
			}
			if cells.Max.X != dedicated.Max.X {
				t.Errorf("cell grid right edge = %d, want %d (dedicated space right edge); %d px black gap before the screen edge",
					cells.Max.X, dedicated.Max.X, dedicated.Max.X-cells.Max.X)
			}
		})
	}
}

// A real Draw paints each row's stripe across the dedicated width.
func TestRowStripesFillDedicatedWidth(t *testing.T) {
	assertDefaultParityState(t)

	dv := newTestDrumView(t, 1280, 720)
	dv.AddRow()
	dv.recalcButtons()
	dv.calcLayout()

	dedicated := dv.widgetRects[WidgetTimeline]
	if dedicated.Empty() {
		t.Fatalf("timeline widget rect empty; layout did not run")
	}

	rowsTop := dv.Bounds.Min.Y + dv.headerH
	rh := dv.rowHeight()

	var stripes []image.Rectangle
	orig := drawRect
	drawRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		// One row tall, in the rows area, in a stripe colour.
		if filled && r.Dy() == rh && r.Min.Y >= rowsTop {
			got := color.RGBAModel.Convert(c).(color.RGBA)
			if got == color.RGBAModel.Convert(genColorDrumStripeEven).(color.RGBA) ||
				got == color.RGBAModel.Convert(genColorDrumStripeOdd).(color.RGBA) {
				stripes = append(stripes, r)
			}
		}
		orig(d, r, c, filled)
	}
	defer func() { drawRect = orig }()

	dst := ebiten.NewImage(1280, 720)
	dv.drawRowsDirect(dst)

	if len(stripes) == 0 {
		t.Fatalf("no row stripes painted; interception missed the draw path")
	}
	for _, s := range stripes {
		if s.Min.X != dedicated.Min.X {
			t.Errorf("row stripe %v starts at x=%d, want %d (dedicated left edge)", s, s.Min.X, dedicated.Min.X)
		}
		if s.Max.X != dedicated.Max.X {
			t.Errorf("row stripe %v ends at x=%d, want %d (dedicated right edge)", s, s.Max.X, dedicated.Max.X)
		}
	}
}
