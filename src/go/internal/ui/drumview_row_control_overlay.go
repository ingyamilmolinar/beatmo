package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

func (dv *DrumView) renderRowControlOverlay(dst *ebiten.Image) {
	dv.drawRowControls(dst)
	// Draw rename via component if available
	if dv.renameComp != nil && dv.renameComp.IsOpen() {
		dv.renameComp.Draw(dst)
	} else if dv.renameBox != nil {
		dv.renameBox.Draw(dst)
	}
	dv.addRowBtn.Draw(dst)
	vis := dv.visibleRows()
	if len(dv.Rows)+1 > vis {
		bar := dv.scrollBarRect()
		drawRect(dst, bar, color.RGBA{80, 80, 80, 255}, true)
		thumb := dv.scrollThumbRect()
		drawRect(dst, thumb, color.RGBA{200, 200, 200, 255}, true)
	}
	// Draw instrument menu via component if available
	if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
		dv.instMenuComp.Draw(dst)
	} else if dv.instMenuOpen {
		// Legacy fallback path
		// Draw menu background expanded to include scrollbar area
		menuBg := dv.instMenuScroll.View
		if menuBg.Empty() {
			menuBg = dv.instMenuFullRect
		}
		if dv.instMenuHasScroll() && !menuBg.Empty() {
			menuBg.Max.X += instMenuScrollBarWidth
		}
		if !menuBg.Empty() {
			drawRect(dst, menuBg, colDropdown, true)
			drawRect(dst, menuBg, colButtonBorder, false)
		}

		if dv.instMenuMode == instMenuModeCategories {
			for _, btn := range dv.instCategoryBtns {
				btn.Draw(dst)
			}
		} else {
			// Render Back + search + instrument items
			for _, btn := range dv.instMenuBtns {
				btn.Draw(dst)
			}
			if dv.instSearchBox != nil {
				// background for search row to avoid empty-looking cell
				drawRect(dst, dv.instSearchRect, colDropdown, true)
				dv.instSearchBox.Draw(dst)
			}
		}
		// Draw scrollbar on top if needed
		if dv.instMenuHasScroll() && !dv.instMenuScroll.View.Empty() {
			bar := dv.instMenuScroll.BarRect(instMenuScrollBarWidth)
			drawRect(dst, bar, color.RGBA{70, 70, 70, 255}, true)
			thumb := dv.instMenuThumbRect()
			drawRect(dst, thumb, color.RGBA{200, 200, 200, 255}, true)
		}
	}
	// Draw subdiv menu via component if available
	if dv.subdivMenuComp != nil && dv.subdivMenuComp.IsOpen() {
		dv.subdivMenuComp.Draw(dst)
	} else if dv.subdivMenuOpen {
		// Legacy fallback path
		for _, btn := range dv.subdivMenuBtns {
			btn.Draw(dst)
		}
	}
	// Draw color wheel via component if available
	if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
		dv.colorWheelComp.Draw(dst)
	} else if dv.colorMenuOpen {
		// Legacy fallback path
		r := dv.colorWheelRect
		if !r.Empty() {
			if dv.colorWheelImg == nil || dv.wheelCacheW != r.Dx() || dv.wheelCacheH != r.Dy() {
				dv.rebuildColorWheelImage()
			}
			if dv.colorWheelImg != nil {
				var op ebiten.DrawImageOptions
				op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
				dst.DrawImage(dv.colorWheelImg, &op)
				drawRect(dst, r, colButtonBorder, false)
			}
		}
	}
	// Draw EQ channel dropdown menu if open
	if dv.eqChannelOpen && len(dv.eqChannelBtns) > 0 {
		menuRect := dv.eqChannelMenuRect()
		// Extend background to include scrollbar when present
		bgRect := menuRect
		if dv.eqChannelScroll.HasScroll() {
			bgRect.Max.X += eqChannelMenuScrollBarWidth
		}
		drawRect(dst, bgRect, colEQBg, true)
		drawRect(dst, bgRect, colButtonBorder, false)
		for _, btn := range dv.eqChannelBtns {
			btn.Draw(dst)
		}
		// Draw scrollbar if needed
		if dv.eqChannelScroll.HasScroll() {
			rowH := dv.rowHeight()
			barRect := dv.eqChannelScroll.BarRect(eqChannelMenuScrollBarWidth)
			drawRect(dst, barRect, color.RGBA{70, 70, 70, 255}, true)
			thumbRect := dv.eqChannelScroll.ThumbRect(eqChannelMenuScrollBarWidth, rowH/2)
			drawRect(dst, thumbRect, color.RGBA{200, 200, 200, 255}, true)
		}
	}
	if dv.naming {
		box := image.Rect(dv.Bounds.Min.X+10, dv.Bounds.Min.Y+110, dv.Bounds.Min.X+300, dv.Bounds.Min.Y+150)
		if dv.nameBox == nil {
			dv.nameBox = NewTextInput(box, BPMBoxStyle)
			dv.nameBox.MaxLen = 32
		}
		dv.nameBox.Rect = box
		dv.nameBox.Draw(dst)
		dv.saveBtn.Draw(dst)
	}
}
