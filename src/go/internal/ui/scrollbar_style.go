package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// A quiet hairline track with the thumb in the chrome accent, so the affordance reads as a live control.
type ScrollbarStyle struct {
	TrackColor color.Color
	ThumbColor color.Color
	Width      int
	MinThumbH  int
	// Every scroller copies its style at construction, so Draw re-reads the CURRENT palette: liveCaptured cannot map the
	// alpha-blended copies. Width and MinThumbH stay per copy.
	live scrollbarLive
}

// scrollbarLive identifies one of the three shared scrollbar palettes.
type scrollbarLive uint8

const (
	scrollbarLiveNone scrollbarLive = iota
	scrollbarLiveDefault
	scrollbarLiveMobile
	scrollbarLiveDropdown
)

func (s ScrollbarStyle) colors() (track, thumb color.Color) {
	switch s.live {
	case scrollbarLiveDefault:
		return DefaultScrollbarStyle.TrackColor, DefaultScrollbarStyle.ThumbColor
	case scrollbarLiveMobile:
		return MobileScrollbarStyle.TrackColor, MobileScrollbarStyle.ThumbColor
	case scrollbarLiveDropdown:
		return DropdownScrollbarStyle.TrackColor, DropdownScrollbarStyle.ThumbColor
	}
	return s.TrackColor, s.ThumbColor
}

// One width for EVERY mobile scrollbar so none reads thicker; the touch target is the 44 px-tall thumb.
const mobileScrollbarWidth = 6

// Derived in rebuildThemeAliases so a runtime theme re-tints them.
var (
	DefaultScrollbarStyle  ScrollbarStyle
	MobileScrollbarStyle   ScrollbarStyle
	DropdownScrollbarStyle ScrollbarStyle
)

func ScrollbarStyleForPlatform() ScrollbarStyle { return Profile().ScrollbarStyle }

// Wider so it is an easy wheel + drag target: on desktop a wheel over a control nudges its value, so rows are
// scrolled from the bar.
const controlGridScrollbarWidth = 16

// Only the gutter widens; the mobile thumb keeps its 44 px height.
func controlGridScrollbarStyle() ScrollbarStyle {
	s := ScrollbarStyleForPlatform()
	s.Width = controlGridScrollbarWidth
	return s
}

// Mobile narrows to the shared width; desktop keeps the wider dropdown affordance.
func dropdownScrollbarStyle() ScrollbarStyle {
	s := DropdownScrollbarStyle
	if Profile().IsMobile() {
		s.Width = mobileScrollbarWidth
	}
	return s
}

// Must match dropdownScrollbarStyle so the reserved space fits the drawn thumb.
func dropdownScrollbarWidth() int {
	if Profile().IsMobile() {
		return mobileScrollbarWidth
	}
	return DropdownScrollbarStyle.Width
}

func (s ScrollbarStyle) Draw(dst *ebiten.Image, barRect, thumbRect image.Rectangle) {
	if barRect.Empty() {
		return
	}
	track, thumb := s.colors()
	drawRect(dst, barRect, track, true)
	if !thumbRect.Empty() {
		drawRect(dst, thumbRect, thumb, true)
	}
}
