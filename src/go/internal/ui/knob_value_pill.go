// src/go/internal/ui/knob_value_pill.go
package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// The mobile knob affordance is a compact, raised, ribbed 3D button showing the
// current value with a "pops up" up-caret. Tapping anywhere in the (larger) knob
// cell opens the vertical scroll-wheel popup. Both the Synth tab and the Sampler
// tab render the identical control on mobile — these two helpers are the single
// source of truth so the two tabs stay pixel-identical.

// knobValuePillRect computes the compact inline pop-up button rect for a knob,
// centered in the control area `cell`, above a caption band of height captionH
// reserved at the bottom of the cell (pass 0 when the caption lives outside the
// cell). Returns the zero rectangle when `cell` is empty.
func knobValuePillRect(cell image.Rectangle, label string, captionH int) image.Rectangle {
	if cell.Empty() {
		return image.Rectangle{}
	}
	const caretW = 9
	btnW := StyledTextWidth(label, RoleBody) + SpaceMD*2 + caretW + SpaceXS
	if maxW := cell.Dx() - SpaceSM*2; btnW > maxW {
		btnW = maxW
	}
	if btnW < 56 {
		btnW = 56
	}
	btnH := knobPillButtonH
	// Center within the control area above the value caption.
	area := image.Rect(cell.Min.X, cell.Min.Y, cell.Max.X, cell.Max.Y-captionH)
	if area.Dy() < btnH+SpaceSM*2 {
		area = cell
	}
	if maxH := area.Dy() - SpaceSM*2; btnH > maxH && maxH > 12 {
		btnH = maxH
	}
	bx := area.Min.X + (area.Dx()-btnW)/2
	by := area.Min.Y + (area.Dy()-btnH)/2
	return image.Rect(bx, by, bx+btnW, by+btnH)
}

// drawKnobValuePillRect draws the raised, ribbed 3D pop-up button at rect r,
// showing `label` (the current value) with a "pops up" up-caret. Token accessors
// only. No-op for an empty rect.
func drawKnobValuePillRect(dst *ebiten.Image, r image.Rectangle, label string) {
	if r.Empty() {
		return
	}
	const caretW = 9
	lw := StyledTextWidth(label, RoleBody)
	rad := RadiusMD / 2
	capFill := TokenSurface3()
	shell := adjustColor(capFill, -45)
	capR := keycapCapRect(r, 0, true) // raised cap (3D pop-up button)

	drawRoundedButton(dst, r, shell, shell, rad, false) // socket / side-wall
	drawKeycapContactShadow(dst, capR, rad)
	drawRoundedButton(dst, capR, capFill, capFill, rad, false) // cap face
	// Analog ribbed texture on the cap (same molded look as the wheel grip).
	if capR.Dx() > 6 && capR.Dy() > 6 {
		spr := wheelGripSprite(capR.Dx()-4, capR.Dy()-4)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(capR.Min.X+2), float64(capR.Min.Y+2))
		dst.DrawImage(spr, op)
	}
	drawKeycapBevel(dst, capR, rad)                                                          // resting cushion
	drawRoundedRect(dst, capR, WithAlphaFromColor(genColorPrimary, AlphaMedium), rad, false) // accent rim

	// Value, embossed (dark drop-shadow under bright glyphs), in the left zone.
	textZoneW := capR.Dx() - caretW - SpaceXS
	lh := StyledTextHeight(RoleBody)
	tx := capR.Min.X + (textZoneW-lw)/2
	if tx < capR.Min.X+SpaceXS {
		tx = capR.Min.X + SpaceXS
	}
	ty := capR.Min.Y + (capR.Dy()-lh)/2
	DrawTextStyled(dst, label, tx, ty+1, RoleBody, WithAlphaFromColor(color.Black, 150))
	DrawTextStyled(dst, label, tx, ty, RoleBody, colTextPrimary)

	// "Pops up" caret at the right edge.
	cyMid := (capR.Min.Y + capR.Max.Y) / 2
	cxr := capR.Max.X - SpaceXS - caretW
	DrawIcon(dst, IconChevronUp, image.Rect(cxr, cyMid-caretW/2, cxr+caretW, cyMid+caretW/2+1),
		WithAlphaFromColor(genColorPrimary, AlphaStrong))
}
