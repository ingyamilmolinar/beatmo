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
	// live names the shared palette this copy was taken from. Every
	// ScrollBehavior/MenuScroll copies a style at construction and keeps it
	// for the menu's lifetime, so Draw re-reads the colors from the CURRENT
	// palette (rebuilt by rebuildThemeAliases on a theme change) instead of
	// the alpha-blended copies, which liveCaptured cannot map. Width and
	// MinThumbH stay per-copy (controlGrid/mobile overrides).
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

// colors returns the track/thumb colors to paint: the live palette's when the
// copy is tagged, else the captured ones.
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

// mobileScrollbarWidth is the single visual thickness shared by EVERY scrollbar
// on mobile — content scrollers (MobileScrollbarStyle) and popup/menu scrollers
// (dropdownScrollbarStyle) alike — so no scrollbar ever reads thicker than
// another on touch. It is intentionally as slim as the desktop hairline; the
// touch target lives in the 44 px-tall thumb (MinThumbH), not the width. See
// DESIGN.md "Mobile vs. Desktop — Intentional Differences".
const mobileScrollbarWidth = 6

// The three styles are DERIVED in rebuildThemeAliases (theme.go) so a runtime
// theme re-tints the thumbs: Default (6px/10), Mobile (mobileScrollbarWidth/44),
// Dropdown (10px/12).
var (
	DefaultScrollbarStyle  ScrollbarStyle
	MobileScrollbarStyle   ScrollbarStyle
	DropdownScrollbarStyle ScrollbarStyle
)

// ScrollbarStyleForPlatform returns the appropriate scrollbar style for the
// current platform (mobile or desktop).
func ScrollbarStyleForPlatform() ScrollbarStyle { return Profile().ScrollbarStyle }

// controlGridScrollbarWidth is the gutter reserved for the Synth/Sampler
// control-grid scrollbars. It is deliberately wider than the 6px app default so
// the bar is an easy WHEEL + DRAG target: on desktop a wheel over a control
// nudges that control's value, so rows are scrolled from the scrollbar itself,
// and a wheel notch anywhere on this widened bar advances the list one row.
const controlGridScrollbarWidth = 16

// controlGridScrollbarStyle is ScrollbarStyleForPlatform widened to
// controlGridScrollbarWidth. The mobile thumb keeps its 44px touch height; only
// the track/gutter thickness grows so the ControlGrid reserves a grabbable lane.
func controlGridScrollbarStyle() ScrollbarStyle {
	s := ScrollbarStyleForPlatform()
	s.Width = controlGridScrollbarWidth
	return s
}

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
	track, thumb := s.colors()
	drawRect(dst, barRect, track, true)
	if !thumbRect.Empty() {
		drawRect(dst, thumbRect, thumb, true)
	}
}
