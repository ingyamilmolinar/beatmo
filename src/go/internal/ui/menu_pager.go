package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// menuPagerFooterH is the reserved footer-band height shared by every list
// menu that shows the page navigator (logs, FX add-menu, template menu). Equal
// to the value the logs pop-up used before extraction (TextHeight()+SpaceMD),
// so logs stays pixel-identical. Density-aware via TextHeight().
func menuPagerFooterH() int { return TextHeight() + SpaceMD }

// menuPagerBtnSize is the prev/next chevron button side length (== the logs
// pop-up's former logsBtnSize()).
func menuPagerBtnSize() int { return TextHeight() + SpaceSM }

// drawMenuPagerFooter draws a centered "Page X / M" label inside footerRect and
// flanks it with the caller-owned prev/next buttons, returning their rects. All
// geometry derives from footerRect (its center-x and bottom edge) so the pager
// adapts to any menu width; the button rects are clamped to stay inside
// footerRect at narrow widths. Pure layout+draw: the caller formats `label`
// (i18n) and owns page state / button OnClick. When dst is nil only the
// geometry is computed (headless tests).
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
