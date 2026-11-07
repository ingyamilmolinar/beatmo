package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Presentational: no hit areas; the owner closes it.
type TooltipOverlay struct {
	text   string
	anchor image.Rectangle
	rect   image.Rectangle
	closed bool
}

// Layout must run before Draw.
func NewTooltipOverlay(text string) *TooltipOverlay {
	return &TooltipOverlay{text: text}
}

// Close marks the overlay for removal on the next portal sweep.
func (o *TooltipOverlay) Close() { o.closed = true }

// Flips above when there is no room below.
func (o *TooltipOverlay) Layout(anchor, screenBounds image.Rectangle) {
	tw := StyledTextWidth(o.text, RoleCaption) + 8
	th := StyledTextHeight(RoleCaption) + 6
	x0 := anchor.Min.X
	y0 := anchor.Max.Y + 2
	if x0+tw > screenBounds.Max.X {
		x0 = screenBounds.Max.X - tw
	}
	if x0 < screenBounds.Min.X {
		x0 = screenBounds.Min.X
	}
	if y0+th > screenBounds.Max.Y {
		// Flip above the anchor.
		y0 = anchor.Min.Y - th - 2
	}
	if y0 < screenBounds.Min.Y {
		y0 = screenBounds.Min.Y
	}
	o.anchor = anchor
	o.rect = image.Rect(x0, y0, x0+tw, y0+th)
}

// HitAreas always returns nil — tooltips do not consume input.
func (o *TooltipOverlay) HitAreas() []HitArea { return nil }

// Draw renders the tooltip background, border, and text.
func (o *TooltipOverlay) Draw(screen *ebiten.Image) {
	if o.rect.Empty() {
		return
	}
	drawRoundedRect(screen, o.rect, colSurface2, RadiusSM, true)
	drawRoundedRect(screen, o.rect, colButtonBorder, RadiusSM, false)
	DrawTextStyled(screen, o.text, o.rect.Min.X+4, o.rect.Min.Y+3, RoleCaption, colTextSecondary)
}

func (o *TooltipOverlay) ShouldClose() bool { return o.closed }
