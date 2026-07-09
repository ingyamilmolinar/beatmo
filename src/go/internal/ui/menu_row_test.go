package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawMenuRow must render every spec variant without panicking.
func TestDrawMenuRowVariants(t *testing.T) {
	dst := ebiten.NewImage(120, 40)
	mk := func() *Button {
		b := NewButton("Kick", DropdownStyle, nil)
		b.SetRect(image.Rect(0, 0, 120, 28))
		return b
	}

	cases := []struct {
		name string
		spec MenuRowSpec
	}{
		{"rest-plain", MenuRowSpec{State: menuItemRest, Label: "Kick"}},
		{"hover-icon", MenuRowSpec{State: menuItemHover, IconID: IconRows, Label: "Bach — Toccata"}},
		{"active-stripe", MenuRowSpec{State: menuItemActive, Accent: colAccent, Label: "8"}},
		{"swatch", MenuRowSpec{State: menuItemRest, Swatch: colAccent, Label: "Snare"}},
		{"centered", MenuRowSpec{State: menuItemActive, CenterLabel: true, Label: "16", LabelColor: colTextAccent}},
		{"highlighted", MenuRowSpec{State: menuItemHover, Highlights: []int{0, 1}, Label: "Kick"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			drawMenuRow(dst, mk(), c.spec) // must not panic
		})
	}
}

// Menus are minimalist-flat (DESIGN.md components note): rows sit directly on
// the single panel surface — no per-row keycap pad, socket, or bevel. The
// keycap is the signature of BUTTONS; list rows carry state via the flat
// accent tint + stripe only. drawMenuRow must therefore never invoke the
// keycap cap-face painter (drawRoundedButton) for any state.
// (2026-07-04 design pass: reverts the keycap chrome the 2026-06-28 row
// unification added, keeping the unified renderer itself.)
func TestDrawMenuRowIsFlatNoKeycap(t *testing.T) {
	origRRB := drawRoundedButton
	defer func() { drawRoundedButton = origRRB }()

	var keycaps int
	drawRoundedButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, radius int, pressed bool) {
		keycaps++
		origRRB(dst, r, fill, border, radius, pressed)
	}

	dst := ebiten.NewImage(120, 40)
	for _, state := range []menuItemState{menuItemRest, menuItemHover, menuItemActive} {
		b := NewButton("opt", DropdownStyle, nil)
		b.SetRect(image.Rect(0, 0, 120, 28))
		drawMenuRow(dst, b, MenuRowSpec{State: state, Label: "opt"})
	}
	if keycaps != 0 {
		t.Fatalf("drawMenuRow painted %d keycap cap-faces; flat menu rows must paint none", keycaps)
	}
}

// The active row still carries the accent indicator: a full-opacity accent
// stripe plus the tinted fill (this is the flat replacement for the old
// keycap+accent stack — and the reason the keycap removal can't regress the
// node dropdown's node-color indicator).
func TestDrawMenuRowActiveDrawsStripeAndTint(t *testing.T) {
	accent := color.RGBA{1, 2, 3, 255} // distinctive; won't collide with chrome colors
	b := NewButton("opt", DropdownStyle, nil)
	b.SetRect(image.Rect(0, 0, 120, 28))

	origRect := drawRect
	defer func() { drawRect = origRect }()

	var stripe, tint bool
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			rgba := color.RGBAModel.Convert(c).(color.RGBA)
			if rgba == accent && r.Dx() == accentStripeW() && r.Dy() == 28 {
				stripe = true
			}
			if rgba.A < 255 && rgba.A > 0 && r.Dx() == 120 && r.Dy() == 28 {
				tint = true
			}
		}
		origRect(dst, r, c, filled)
	}

	dst := ebiten.NewImage(120, 40)
	drawMenuRow(dst, b, MenuRowSpec{Accent: accent, State: menuItemActive, Label: "opt"})

	if !stripe {
		t.Fatal("active row drew no accent stripe")
	}
	if !tint {
		t.Fatal("active row drew no tinted fill")
	}
}

// Hover/press feedback on a flat row is the accent tint over the full row.
func TestDrawMenuRowHoverDrawsTint(t *testing.T) {
	accent := color.RGBA{4, 5, 6, 255}
	b := NewButton("opt", DropdownStyle, nil)
	b.SetRect(image.Rect(0, 0, 120, 28))

	origRect := drawRect
	defer func() { drawRect = origRect }()

	var tint bool
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		// A translucent full-row fill is the hover tint (alpha-composited
		// from the accent; premultiplication scrambles the RGB so only the
		// translucency + geometry are asserted).
		rgba := color.RGBAModel.Convert(c).(color.RGBA)
		if filled && rgba.A > 0 && rgba.A < 255 && r.Dx() == 120 && r.Dy() == 28 {
			tint = true
		}
		origRect(dst, r, c, filled)
	}

	dst := ebiten.NewImage(120, 40)
	drawMenuRow(dst, b, MenuRowSpec{Accent: accent, State: menuItemHover, Label: "opt"})

	if !tint {
		t.Fatal("hovered row drew no accent tint")
	}
}
