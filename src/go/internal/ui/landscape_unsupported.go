package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Mobile landscape is unsupported on purpose: a ~390 px-tall screen cannot hold a usable grid AND the audio
// panel, so a rotate notice replaces the UI. Layout state is untouched, so rotating back resumes seamlessly.

// Test seam, nil in production: asserts the notice path without reading pixels.
var landscapeNoticeDrawHook func()

// Wide desktop windows are always supported.
func (g *Game) landscapeUnsupported() bool {
	return Profile().IsMobile() && g.winW > g.winH
}

func landscapeNoticeLines() []string {
	return []string{
		i18n.T(i18n.KeyOrientationTitle),
		i18n.T(i18n.KeyOrientationBody),
	}
}

func (g *Game) drawLandscapeUnsupported(screen *ebiten.Image) {
	if landscapeNoticeDrawHook != nil {
		landscapeNoticeDrawHook()
	}
	b := screen.Bounds()
	drawRect(screen, b, colBGTop, true)

	lines := landscapeNoticeLines()
	if len(lines) == 0 {
		return
	}
	roles := []TextRole{RolePanelTitle, RoleBody}
	cols := []color.Color{colTextPrimary, colTextSecondary}

	// Total stacked height (title + gap + body) to vertically center the block.
	gap := SpaceMD
	total := 0
	for i := range lines {
		total += StyledTextHeight(roles[i])
		if i > 0 {
			total += gap
		}
	}
	y := b.Min.Y + (b.Dy()-total)/2
	cx := b.Min.X + b.Dx()/2
	for i, ln := range lines {
		w := StyledTextWidth(ln, roles[i])
		DrawTextStyled(screen, ln, cx-w/2, y, roles[i], cols[i])
		y += StyledTextHeight(roles[i]) + gap
	}
}
