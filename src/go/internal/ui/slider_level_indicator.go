package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// indicatorTrackHeight is the thickness of the slider-level marker line.
// Two pixels reads as a deliberate underline at every supported icon size
// without competing with the glyph itself.
const indicatorTrackHeight = 2

// indicatorGapBelowIcon is the vertical breathing room between the icon
// glyph's bottom edge and the indicator's top edge. The indicator may
// shrink/shift up if the button rect is too tight, but never below 1px.
const indicatorGapBelowIcon = 3

// indicatorMinIconWidth is the smallest icon footprint that still gets an
// indicator. Below this the bar is not legible.
const indicatorMinIconWidth = 8

// drawSliderLevelIndicator draws a thin underline beneath an icon button to
// show the 0..1 level of the slider that the icon toggles. The bar spans the
// icon glyph's horizontal footprint and sits a few pixels below it, but is
// always clamped inside btnR so dense toolbars (e.g., master volume on
// desktop) cannot clip it. Track is dim; fill carries the icon's foreground
// color at full alpha. When off (muted/zero), the track renders alone —
// communicating "this control exists, currently silent."
//
// btnR is the button hit-area; iconR is the centered glyph rect inside it.
// Value is clamped to [0,1]. Empty/too-small iconR is a no-op. If btnR has
// no room below iconR for even a 1px track, the indicator is also a no-op
// (the layout would clip it anyway).
func drawSliderLevelIndicator(dst *ebiten.Image, btnR, iconR image.Rectangle, value float64, fgCol color.Color, off bool) {
	if iconR.Empty() {
		return
	}
	width := iconR.Dx()
	if width < indicatorMinIconWidth {
		return
	}
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}

	// Position below the icon, clamped so the bar always fits inside btnR.
	maxY := iconR.Max.Y + indicatorGapBelowIcon + indicatorTrackHeight
	if !btnR.Empty() && maxY > btnR.Max.Y {
		maxY = btnR.Max.Y
	}
	y1 := maxY
	y0 := y1 - indicatorTrackHeight
	if y0 <= iconR.Max.Y {
		// No gap left between icon and bar; pull bar down to sit flush below the icon.
		y0 = iconR.Max.Y
		y1 = y0 + indicatorTrackHeight
		if !btnR.Empty() && y1 > btnR.Max.Y {
			y1 = btnR.Max.Y
		}
	}
	if y1-y0 < 1 {
		return
	}

	trackR := image.Rect(iconR.Min.X, y0, iconR.Max.X, y1)
	drawRect(dst, trackR, WithAlphaFromColor(fgCol, AlphaSubtle), true)

	if off || value <= 0 {
		return
	}
	fillW := int(math.Round(float64(width) * value))
	if fillW <= 0 {
		return
	}
	if fillW > width {
		fillW = width
	}
	fillR := image.Rect(iconR.Min.X, y0, iconR.Min.X+fillW, y1)
	drawRect(dst, fillR, fgCol, true)
}

// drawSliderLevelIndicatorBounded is an alias retained for tests that name
// the bounded behavior explicitly. Forwards to drawSliderLevelIndicator.
func drawSliderLevelIndicatorBounded(dst *ebiten.Image, btnR, iconR image.Rectangle, value float64, fgCol color.Color, off bool) {
	drawSliderLevelIndicator(dst, btnR, iconR, value, fgCol, off)
}

// drawVolumeButton renders the canonical volume control: a centered speaker
// glyph (on/off variant by mute or zero level) plus the at-a-glance level
// indicator beneath it. This is the SINGLE source of truth for both the
// per-row volume cells AND the master volume button — they differ only in
// which channel they're wired to. Any visual tweak (icon size, indicator
// thickness, off-state behavior) propagates to both surfaces automatically.
//
// btnR is the button hit-area; the speaker glyph is centered inside it at
// 60% height (clamped on mobile to IconSizeMD). rowColor optionally tints
// the glyph (per-row uses the row's instrument color); pass nil for the
// master button to fall back to colVolumeIconOn.
func drawVolumeButton(dst *ebiten.Image, btnR image.Rectangle, vol float64, muted bool, rowColor color.Color) {
	if btnR.Empty() {
		return
	}
	cellH := btnR.Dy()
	iconH := cellH * 60 / 100
	if Profile().IsMobile() && iconH < IconSizeMD {
		iconH = IconSizeMD
	}
	if iconH < 8 {
		iconH = 8
	}
	cx := btnR.Min.X + btnR.Dx()/2
	cy := btnR.Min.Y + btnR.Dy()/2
	iconR := image.Rect(cx-iconH/2, cy-iconH/2, cx+iconH/2, cy+iconH/2)

	iconCol := volIconColor(vol, muted, rowColor)
	glyph := IconSpeaker
	if vol <= 0 || muted {
		glyph = IconSpeakerOff
	}
	DrawIcon(dst, glyph, iconR, iconCol)
	drawSliderLevelIndicator(dst, btnR, iconR, vol, iconCol, muted || vol <= 0)
}
