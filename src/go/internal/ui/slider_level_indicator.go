package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Below this the strip is not legible.
const indicatorMinWidth = 8

// Empty when the button is too small to carry one.
func volumeGaugeRect(btnR image.Rectangle) image.Rectangle {
	if btnR.Dx() < indicatorMinWidth {
		return image.Rectangle{}
	}
	gh := Profile().DensityValues().ParamCardGaugeH
	if gh < 2 {
		gh = 2
	}
	side := SpaceSM
	if q := btnR.Dx() / 4; q < side {
		side = q
	}
	y1 := btnR.Max.Y - SpaceXS
	y0 := y1 - gh
	if y0 <= btnR.Min.Y {
		return image.Rectangle{}
	}
	return image.Rect(btnR.Min.X+side, y0, btnR.Max.X-side, y1)
}

// The shared gauge renderer; a muted/zero level shows the track alone: "this control exists, currently silent".
// iconR only guards "is there a glyph".
func drawSliderLevelIndicator(dst *ebiten.Image, btnR, iconR image.Rectangle, value float64, fgCol color.Color, off bool) {
	if iconR.Empty() {
		return
	}
	gr := volumeGaugeRect(btnR)
	if gr.Empty() {
		return
	}
	v := clamp01(value)
	if off {
		v = 0
	}
	drawGaugeStrip(dst, gr, paramCardGauge{kind: gaugeFill, frac: v}, fgCol)
}

// The SINGLE renderer for row and master volume, so a visual tweak reaches both. rowColor tints glyph and gauge;
// nil for master.
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
	// Center the glyph in the band above the gauge so the two never touch.
	bandBottom := btnR.Max.Y
	if gr := volumeGaugeRect(btnR); !gr.Empty() {
		bandBottom = gr.Min.Y
	}
	if band := bandBottom - btnR.Min.Y; iconH > band-2 && band > 8 {
		iconH = band - 2
	}
	cx := btnR.Min.X + btnR.Dx()/2
	cy := btnR.Min.Y + (bandBottom-btnR.Min.Y)/2
	iconR := image.Rect(cx-iconH/2, cy-iconH/2, cx+iconH/2, cy+iconH/2)

	// The keycap body every neighbouring control shares; no hover/press tint, since a press opens the popup. fitRadius:
	// narrow 14 px mobile cells.
	spec := Spec(ComponentButtonSecondary)
	spec.Radius = fitRadius(spec.Radius, btnR)
	Render(dst, btnR, spec, ComponentState{})

	iconCol := volIconColor(vol, muted, rowColor)
	glyph := IconSpeaker
	if vol <= 0 || muted {
		glyph = IconSpeakerOff
	}
	DrawIcon(dst, glyph, iconR, iconCol)
	drawSliderLevelIndicator(dst, btnR, iconR, vol, iconCol, muted || vol <= 0)
}
