package ui

import (
	"image/color"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// themePickerID identifies the swatch-picker portal entry opened from a theme
// chip. It STACKS on the open theme overlay, so it is opened as a must-answer
// (modal) entry: the HitIndex modal filter then routes nothing to the panel
// beneath, and a tap anywhere outside the picker dismisses just the picker
// (tree CloseTop) instead of acting on the theme rows. Closing it restores the
// overlay's non-modal state (OverlayPortal.updateModalState).
const themePickerID = "theme-swatch"

// openThemeSwatchPicker opens the shared swatch grid for a theme category.
// The pick writes the swatch NAME into the overlay's active scope and applies
// live; the picker closes on release (ColorWheelComponent latches `picked`),
// the overlay stays open so the user keeps tweaking.
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
		// The editor is window-bounded (theme_overlay.go), so the picker may
		// use the whole window too.
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

// swatchNameOf maps a picked color back to its curated swatch name (RGB only;
// the picker returns opaque palette colors).
func swatchNameOf(c color.Color) (string, bool) {
	r, gg, b, _ := c.RGBA()
	for _, s := range SuggestedInstrumentSwatches {
		if s.RGBA.R == uint8(r>>8) && s.RGBA.G == uint8(gg>>8) && s.RGBA.B == uint8(b>>8) {
			return s.Name, true
		}
	}
	return "", false
}
