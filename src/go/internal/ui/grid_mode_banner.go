package ui

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// gridModeBannerText returns the localized banner string for the currently
// active armed grid mode (move / copy / connect), or "" when idle
// (GridModeNone). Single source of truth shared by the draw path and tests —
// pure, and deliberately excludes the Esc-hint suffix, which is the draw
// path's job (drawGridModeBanner) since it depends on Profile().ShowEscHint.
func (g *Game) gridModeBannerText() string {
	s := g.modes.Subject()
	switch g.modes.Active() {
	case GridModeMove:
		if s == nil {
			return ""
		}
		return fmt.Sprintf(i18n.T(i18n.KeyBannerMoving), s.I, s.J)
	case GridModeCopy:
		return i18n.T(i18n.KeyBannerPasting)
	case GridModeConnect:
		if s == nil {
			return ""
		}
		return fmt.Sprintf(i18n.T(i18n.KeyBannerConnecting), s.I, s.J)
	}
	return ""
}

// drawGridModeBanner renders the armed-mode helper banner centered at the top
// of the grid pane: RoleBody chrome text on a panel, sized from the role's
// metrics and spacing tokens per DESIGN.md's typography roles (no bitmap
// font, no magic paddings). Returns early when idle. Single shared draw site
// for Move, Copy and Connect — replaces the three former bespoke banners
// (Task 9).
func (g *Game) drawGridModeBanner(dst *ebiten.Image) {
	text := g.gridModeBannerText()
	if text == "" {
		return
	}
	if Profile().ShowEscHint {
		text += "  " + i18n.T(i18n.KeyEscToCancel)
	}
	w := StyledTextWidth(text, RoleBody) + 2*SpaceMD
	gridW := g.split.GridW(g.winW)
	x := (gridW - w) / 2
	if x < 0 {
		x = 0
	}
	rect := image.Rect(x, SpaceXS, x+w, SpaceXS+StyledTextHeight(RoleBody)+2*SpaceXS)
	// Keep clear of the settings gear's protected top-right zone (same
	// no-invade contract as the long-press popup — see
	// feedback_settings_gear_protected_zone).
	rect = avoidReservedRect(rect, g.gridTopButtonsReservedRect(), gridW, g.split.GridH(g.winH))
	drawPanel(dst, rect)
	DrawTextStyled(dst, text, rect.Min.X+SpaceMD, rect.Min.Y+SpaceXS, RoleBody, colTextPrimary)
}
