package ui

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// gridModeBannerText builds the localized helper-banner line for an armed
// grid mode. Pure — tests and the draw path share this one source. escHint
// appends the desktop-only " (Esc to cancel)" suffix (Profile().ShowEscHint
// at the call site).
func gridModeBannerText(kind GridModeKind, subject *uiNode, escHint bool) string {
	var s string
	switch kind {
	case GridModeMove:
		s = fmt.Sprintf(i18n.T(i18n.KeyBannerMoving), subject.I, subject.J)
	case GridModeCopy:
		s = i18n.T(i18n.KeyBannerPasting)
	case GridModeConnect:
		s = fmt.Sprintf(i18n.T(i18n.KeyBannerConnectFrom), subject.I, subject.J)
	default:
		return ""
	}
	if escHint {
		s += i18n.T(i18n.KeyBannerEscHint)
	}
	return s
}

// drawGridModeBanner renders an armed-mode helper banner centered at the top
// of the grid pane: RoleBody chrome text on a Surface-2 rounded panel, sized
// from the role's metrics and spacing tokens per DESIGN.md's typography
// roles (no bitmap font, no magic paddings).
func (g *Game) drawGridModeBanner(dst *ebiten.Image, text string) {
	if text == "" {
		return
	}
	txtW := StyledTextWidth(text, RoleBody)
	txtH := StyledTextHeight(RoleBody)
	w := txtW + 2*SpaceSM
	h := txtH + 2*SpaceXS
	gridW := g.split.GridW(g.winW)
	x := (gridW - w) / 2
	if x < 0 {
		x = 0
	}
	r := image.Rect(x, SpaceXS, x+w, SpaceXS+h)
	// Keep clear of the settings gear's protected top-right zone (same
	// no-invade contract as the long-press popup).
	r = avoidReservedRect(r, g.gridHelpReservedRect(), gridW, g.split.GridH(g.winH))
	drawRoundedRect(dst, r, TokenSurface2(), popupCornerRadius(), true)
	drawRoundedRect(dst, r, TokenBorderMedium(), popupCornerRadius(), false)
	DrawTextStyled(dst, text, r.Min.X+SpaceSM, r.Min.Y+SpaceXS, RoleBody, TokenTextPrimary())
}
