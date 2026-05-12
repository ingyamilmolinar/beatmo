package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// TooltipOverlay is a lightweight, decorative portal overlay that renders a
// caption-scaled tooltip below an anchor rectangle. It is used by the
// ChainPanelZone hover-dwell flow (and any other UI that needs a passive
// tooltip on a control). The overlay never registers hit areas — it is purely
// presentational and dismissed by the owner via Close() (or by ShouldClose()
// being polled by the OverlayPortal).
type TooltipOverlay struct {
	text   string
	anchor image.Rectangle
	rect   image.Rectangle
	closed bool
}

// NewTooltipOverlay builds a tooltip displaying text. Layout must be called
// before Draw to position the rect.
func NewTooltipOverlay(text string) *TooltipOverlay {
	return &TooltipOverlay{text: text}
}

// Close marks the overlay for removal on the next portal sweep.
func (o *TooltipOverlay) Close() { o.closed = true }

// Layout positions the tooltip just below the anchor, clamping to
// screenBounds and flipping above when there is no room below.
func (o *TooltipOverlay) Layout(anchor, screenBounds image.Rectangle) {
	captionScale := FontSizeCaption / FontSizeBody
	tw := int(float64(TextWidth(o.text))*captionScale) + 8
	th := int(float64(TextHeight())*captionScale) + 6
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
	captionScale := FontSizeCaption / FontSizeBody
	DrawTextColorAtScale(screen, o.text, o.rect.Min.X+4, o.rect.Min.Y+3, colTextSecondary, captionScale)
}

// ShouldClose reports whether Close() has been invoked. Polled by
// OverlayPortal.CleanupClosed each frame.
func (o *TooltipOverlay) ShouldClose() bool { return o.closed }
