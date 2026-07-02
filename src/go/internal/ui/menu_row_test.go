package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawMenuRow must render every spec variant without panicking and must
// advance the button's press animation (it draws the keycap via btn.Draw).
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

// drawMenuRow must tick the press animation: a button drawn while its press
// target is engaged advances pressDepth toward 1 across successive draws.
func TestDrawMenuRowAdvancesPressAnim(t *testing.T) {
	dst := ebiten.NewImage(120, 40)
	b := NewButton("Kick", DropdownStyle, nil)
	b.SetRect(image.Rect(0, 0, 120, 28))
	b.pressTarget = 1 // simulate a held press
	start := b.pressDepth
	for i := 0; i < 5; i++ {
		drawMenuRow(dst, b, MenuRowSpec{State: menuItemHover, Label: "Kick"})
	}
	if b.pressDepth <= start {
		t.Fatalf("drawMenuRow did not advance press animation: depth %v <= start %v", b.pressDepth, start)
	}
}

// TestDrawMenuRowAccentDrawnOverKeycap is a z-order regression guard: the
// per-state accent (active stripe/tint) MUST be painted AFTER the keycap shell,
// or the opaque keycap (drawKeycapShell → drawRoundedButton over the full row)
// covers it and the accent becomes invisible. This is exactly the bug that hid
// the node dropdown's node-color indicator (menus with a swatch/accent-label
// masked it). We intercept drawRoundedButton (keycap) and drawRect (accent
// stripe) and assert the accent stripe is recorded after the first keycap draw.
func TestDrawMenuRowAccentDrawnOverKeycap(t *testing.T) {
	accent := color.RGBA{1, 2, 3, 255} // distinctive; won't collide with chrome colors
	b := NewButton("opt", DropdownStyle, nil)
	b.SetRect(image.Rect(0, 0, 120, 28))

	origRRB := drawRoundedButton
	origRect := drawRect
	defer func() { drawRoundedButton = origRRB; drawRect = origRect }()

	var seq []string
	drawRoundedButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, radius int, pressed bool) {
		seq = append(seq, "keycap")
		origRRB(dst, r, fill, border, radius, pressed)
	}
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled && color.RGBAModel.Convert(c).(color.RGBA) == accent {
			seq = append(seq, "accent")
		}
		origRect(dst, r, c, filled)
	}

	dst := ebiten.NewImage(120, 40)
	drawMenuRow(dst, b, MenuRowSpec{Accent: accent, State: menuItemActive, Label: "opt"})

	firstKeycap, accentIdx := -1, -1
	for i, e := range seq {
		if e == "keycap" && firstKeycap == -1 {
			firstKeycap = i
		}
		if e == "accent" {
			accentIdx = i
		}
	}
	if firstKeycap == -1 {
		t.Fatal("no keycap (drawRoundedButton) call recorded; cannot verify draw order")
	}
	if accentIdx == -1 {
		t.Fatal("active accent stripe was never drawn")
	}
	if accentIdx < firstKeycap {
		t.Fatalf("accent stripe drawn BEFORE keycap (idx %d < %d) — the opaque keycap will cover the accent", accentIdx, firstKeycap)
	}
}
