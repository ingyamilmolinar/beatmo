//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The channel pill is a SELECTOR (identity), not a latched tab (state) —
// it must not render in the same lit-amber active treatment as the tab
// pills, or every screen reads as having two active tabs and the amber
// "this is on" signal dilutes (D2 in the 2026-07-04 critique: mobile
// Chain showed five gold actives at once). The pill renders as a neutral
// dropdown control; the accent chevron + tinted border carry the
// "opens a list" affordance (button-dropdown recipe).
func TestChannelPillIsNotLatchedAmber(t *testing.T) {
	bar := NewAudioStickyBar(130, func() {}, func(PanelTab) {})
	bar.Layout(image.Rect(0, 0, 800, 30))

	chRect := bar.ChannelBtn().Rect()
	if chRect.Empty() {
		t.Fatal("channel pill has no rect after Layout")
	}

	origRRB := drawRoundedButton
	defer func() { drawRoundedButton = origRRB }()

	amber := color.RGBAModel.Convert(pillCapFillActive).(color.RGBA)
	var amberOnChannelPill bool
	drawRoundedButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, radius int, pressed bool) {
		if r.In(chRect) && fill != nil {
			if color.RGBAModel.Convert(fill).(color.RGBA) == amber {
				amberOnChannelPill = true
			}
		}
		origRRB(dst, r, fill, border, radius, pressed)
	}

	dst := ebiten.NewImage(800, 30)
	bar.Draw(dst, TabEQ)

	if amberOnChannelPill {
		t.Fatal("channel pill rendered with the latched-amber active fill; it must read as a neutral dropdown selector")
	}
}
