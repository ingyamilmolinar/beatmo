//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A selector, not a latched tab: lit amber would make every screen read as having two active tabs. Neutral, with the
// accent chevron and border saying "opens a list".
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
