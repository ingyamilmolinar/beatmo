package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Shared by every paged list menu; equals the logs pop-up's original band, so logs stays
// pixel-identical.
func menuPagerFooterH() int { return TextHeight() + SpaceMD }

func menuPagerBtnSize() int { return TextHeight() + SpaceSM }

// All geometry derives from footerRect, clamped at narrow widths; the caller owns label, page state
// and OnClick. dst nil computes geometry only.
func drawMenuPagerFooter(dst *ebiten.Image, footerRect image.Rectangle, label string, prev, next *Button) (prevRect, nextRect image.Rectangle) {
	lw := TextWidth(label)
	fy := footerRect.Max.Y - menuPagerFooterH() + SpaceXS
	cx := footerRect.Min.X + footerRect.Dx()/2
	bsz := menuPagerBtnSize()
	by := fy + (TextHeight()-bsz)/2

	prevRect = image.Rect(cx-lw/2-SpaceLG-bsz, by, cx-lw/2-SpaceLG, by+bsz)
	nextRect = image.Rect(cx+lw/2+SpaceLG, by, cx+lw/2+SpaceLG+bsz, by+bsz)

	// Clamp horizontally so neither button escapes a narrow footer.
	if prevRect.Min.X < footerRect.Min.X {
		dx := footerRect.Min.X - prevRect.Min.X
		prevRect = prevRect.Add(image.Pt(dx, 0))
	}
	if nextRect.Max.X > footerRect.Max.X {
		dx := nextRect.Max.X - footerRect.Max.X
		nextRect = nextRect.Sub(image.Pt(dx, 0))
	}

	if dst == nil {
		return prevRect, nextRect
	}
	DrawTextColorAt(dst, label, cx-lw/2, fy, colTextSecondary)
	prev.SetRect(prevRect)
	next.SetRect(nextRect)
	prev.Draw(dst)
	next.Draw(dst)
	return prevRect, nextRect
}
