package ui

import (
	"image/color"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// Stacked on the open overlay as must-answer, so the modal filter routes nothing to the panel beneath and an outside
// tap dismisses just the picker.
const themePickerID = "theme-swatch"

// The picker closes on release; the overlay stays open so the user keeps tweaking.
func (g *Game) openThemeSwatchPicker(ov *ThemeOverlay, categoryID string) {
	dv := g.drum
	if dv == nil || dv.tree == nil || dv.colorWheelComp == nil || ov == nil {
		return
	}
	p := dv.portal()
	if p == nil {
		return
	}
	if p.Has(themePickerID) {
		p.Close(themePickerID)
	}
	anchor := ov.ChipRect(categoryID)
	scope := ov.Scope()
	dv.colorWheelComp.SetProps(ColorWheelProps{
		AnchorRect: anchor,
		// The editor is window-bounded, so the picker may use the window too.
		Bounds:         g.windowRect(),
		RowHeight:      dv.rowHeight(),
		CurrentColorFn: func() color.Color { return ov.chipSwatch(categoryID) },
		OnColorPick: func(c color.Color) {
			name, ok := swatchNameOf(c)
			if !ok {
				return
			}
			if scope == hooks.ThemeScopeCircuit {
				g.setCircuitThemeCategory(categoryID, name)
			} else {
				g.setUserThemeCategory(categoryID, name)
			}
		},
		OnClose: func() {},
	})
	dv.colorWheelComp.Open()
	dv.colorWheelComp.ClearHold()
	p.Open(menuCardEntry(themePickerID,
		&compPortalOverlay{comp: dv.colorWheelComp, tag: "theme-swatch", updateFn: dv.colorWheelComp.UpdateKeyNav},
		anchor, true /* stacked on an open menu → must-answer, see themePickerID */, func() {
			if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
				dv.colorWheelComp.Close()
			}
		}))
}

// RGB only: the picker returns opaque palette colours.
func swatchNameOf(c color.Color) (string, bool) {
	r, gg, b, _ := c.RGBA()
	for _, s := range SuggestedInstrumentSwatches {
		if s.RGBA.R == uint8(r>>8) && s.RGBA.G == uint8(gg>>8) && s.RGBA.B == uint8(b>>8) {
			return s.Name, true
		}
	}
	return "", false
}
