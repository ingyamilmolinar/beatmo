package ui

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Pure; the Esc hint is the draw path's job because it depends on the profile.
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

// The one draw site for Move, Copy and Connect.
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
	// Never invades the top-right button zone.
	rect = avoidReservedRect(rect, g.gridTopButtonsReservedRect(), gridW, g.split.GridH(g.winH))
	drawPanel(dst, rect)
	DrawTextStyled(dst, text, rect.Min.X+SpaceMD, rect.Min.Y+SpaceXS, RoleBody, colTextPrimary)
}
