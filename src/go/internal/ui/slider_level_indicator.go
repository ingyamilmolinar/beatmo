package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// indicatorMinWidth is the narrowest button that still gets a level gauge.
// Below this the strip is not legible.
const indicatorMinWidth = 8

// volumeGaugeRect is the DESIGN.md "gauge" strip of a volume button: a
// hairline (densities.paramCardGaugeH) on the body's bottom inner edge,
// spanning the body width minus a side inset. Empty when the button is too
// small to carry one.
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

// drawSliderLevelIndicator draws the at-a-glance level gauge of a volume
// button: the shared gauge strip (drawGaugeStrip — the same renderer the
// Synth param cards use) along the bottom inner edge of btnR. The fill
// carries fgCol (the instrument color on a row, the volume glyph color on
// master); when off (muted/zero) the track renders alone — "this control
// exists, currently silent." Value is clamped to [0,1]; a button narrower
// than indicatorMinWidth gets no gauge. iconR is accepted for the callers'
// symmetry and as the "is there a glyph" guard.
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

// drawVolumeButton renders the canonical volume control: a speaker glyph
// (on/off variant by mute or zero level) centered above the level gauge. This
// is the SINGLE source of truth for both the per-row volume cells AND the
// master volume button — they differ only in which channel they're wired to.
// Any visual tweak (icon size, gauge, off-state behavior) propagates to both
// surfaces automatically.
//
// btnR is the button hit-area; the speaker glyph is 60% of its height
// (clamped on mobile to IconSizeMD), centered in the band above the gauge.
// rowColor optionally tints the glyph and gauge (per-row uses the row's
// instrument color); pass nil for the master button to fall back to
// colVolumeIconOn.
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

	// Keycap body first: the volume button is a pressable control and shares
	// the button-secondary spec body with every neighbouring keycap (transport
	// Play/Stop/undo, the per-row M/S/FX cells). Static state — the volume hit
	// adapter opens the popup on press, so there is no hover/press tint here.
	// Guard: TestVolumeButtonsAndInputsShareKeycapBody.
	// fitRadius: narrow mobile cells (14 px wide) fall under the anti-pill
	// ceiling min(w,h)/4 at the spec's radius (TestNoPillShapedControls).
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
