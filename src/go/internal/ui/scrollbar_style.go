package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// ScrollbarStyle defines visual properties for a scrollbar track and thumb.
type ScrollbarStyle struct {
	TrackColor color.Color
	ThumbColor color.Color
	Width      int
	MinThumbH  int
}

var (
	DefaultScrollbarStyle = ScrollbarStyle{
		TrackColor: color.NRGBA{255, 255, 255, 8},
		ThumbColor: color.NRGBA{255, 255, 255, 40},
		Width:      6,
		MinThumbH:  10,
	}
	MobileScrollbarStyle = ScrollbarStyle{
		TrackColor: color.NRGBA{255, 255, 255, 8},
		ThumbColor: color.NRGBA{255, 255, 255, 50},
		Width:      16,
		MinThumbH:  44,
	}
	DropdownScrollbarStyle = ScrollbarStyle{
		TrackColor: color.NRGBA{255, 255, 255, 8},
		ThumbColor: color.NRGBA{255, 255, 255, 40},
		Width:      10,
		MinThumbH:  12,
	}
)

// ScrollbarStyleForPlatform returns the appropriate scrollbar style for the
// current platform (mobile or desktop).
func ScrollbarStyleForPlatform() ScrollbarStyle { return Profile().ScrollbarStyle }

// Draw renders the scrollbar track and thumb. It is stateless — it draws
// whatever rectangles are given.
func (s ScrollbarStyle) Draw(dst *ebiten.Image, barRect, thumbRect image.Rectangle) {
	if barRect.Empty() {
		return
	}
	drawRect(dst, barRect, s.TrackColor, true)
	if !thumbRect.Empty() {
		drawRect(dst, thumbRect, s.ThumbColor, true)
	}
}
