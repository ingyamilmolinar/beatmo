package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// ScrollbarStyle defines visual properties for a scrollbar track and thumb.
// The track is a neutral white-base hairline (`border-thin`, 8/255 — same
// strength as section dividers) so the chassis stays quiet, while the thumb
// is the sunset-gold chrome accent (`primary`) composited at the
// `scrollbar-thumb` / `scrollbar-thumb-mobile` buckets (40 / 50). Painting
// the thumb in the accent makes the scroll affordance read as a live,
// interactive control in the same gold language as every other active
// chrome surface.
type ScrollbarStyle struct {
	TrackColor color.Color
	ThumbColor color.Color
	Width      int
	MinThumbH  int
}

// mobileScrollbarWidth is the single visual thickness shared by EVERY scrollbar
// on mobile — content scrollers (MobileScrollbarStyle) and popup/menu scrollers
// (dropdownScrollbarStyle) alike — so no scrollbar ever reads thicker than
// another on touch. It is intentionally as slim as the desktop hairline; the
// touch target lives in the 44 px-tall thumb (MinThumbH), not the width. See
// DESIGN.md "Mobile vs. Desktop — Intentional Differences".
const mobileScrollbarWidth = 6

var (
	DefaultScrollbarStyle = ScrollbarStyle{
		TrackColor: WithAlpha(genColorBorder, genAlphaBorderThin),
		ThumbColor: WithAlpha(genColorPrimary, genAlphaScrollbarThumb),
		Width:      6,
		MinThumbH:  10,
	}
	MobileScrollbarStyle = ScrollbarStyle{
		TrackColor: WithAlpha(genColorBorder, genAlphaBorderThin),
		ThumbColor: WithAlpha(genColorPrimary, genAlphaScrollbarThumbMobile),
		Width:      mobileScrollbarWidth,
		MinThumbH:  44,
	}
	DropdownScrollbarStyle = ScrollbarStyle{
		TrackColor: WithAlpha(genColorBorder, genAlphaBorderThin),
		ThumbColor: WithAlpha(genColorPrimary, genAlphaScrollbarThumb),
		Width:      10,
		MinThumbH:  12,
	}
)

// ScrollbarStyleForPlatform returns the appropriate scrollbar style for the
// current platform (mobile or desktop).
func ScrollbarStyleForPlatform() ScrollbarStyle { return Profile().ScrollbarStyle }

// dropdownScrollbarStyle returns the scrollbar style for popup/menu scrollers
// (instrument menu, EQ-channel dropdown, context/overflow/subdivision menus).
// On mobile it narrows to mobileScrollbarWidth so every mobile scrollbar shares
// one width; desktop keeps the slightly wider dropdown affordance.
func dropdownScrollbarStyle() ScrollbarStyle {
	s := DropdownScrollbarStyle
	if Profile().IsMobile() {
		s.Width = mobileScrollbarWidth
	}
	return s
}

// dropdownScrollbarWidth is the width reserved for popup/menu scrollbars in
// layout math (button right-inset, thumb hit rect). It mirrors
// dropdownScrollbarStyle so the reserved space matches the drawn thumb.
func dropdownScrollbarWidth() int {
	if Profile().IsMobile() {
		return mobileScrollbarWidth
	}
	return DropdownScrollbarStyle.Width
}

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
