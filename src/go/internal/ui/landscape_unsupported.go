package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Mobile landscape is intentionally unsupported. The portrait layout assumes a
// tall framebuffer (the audio panel alone wants ~350px of height); in landscape
// the ~390px-tall screen cannot host both a usable grid AND the audio panel, so
// the panel's content overlaps or clips. Rather than ship a broken layout, the
// mobile profile detects landscape and renders a full-screen "rotate to
// portrait" notice. Detection and rendering both live here so the disable is a
// single, well-tested seam; rotating back to portrait simply makes
// landscapeUnsupported() false again and the normal UI resumes (Layout state is
// untouched by the notice, so the transition is seamless).

// landscapeNoticeDrawHook is a test-only observation seam (nil in production),
// mirroring drawTextColorAtScaleHook. drawLandscapeUnsupported invokes it so a
// test can assert the notice path was taken without reading pixels.
var landscapeNoticeDrawHook func()

// landscapeUnsupported reports whether the current frame is a mobile (small
// screen) profile in landscape orientation (width > height). Only the mobile
// profile disables landscape; wide desktop windows are always supported.
func (g *Game) landscapeUnsupported() bool {
	return Profile().IsMobile() && g.winW > g.winH
}

// landscapeNoticeLines returns the localized lines of the rotate-to-portrait
// notice (title first, then the explanatory body).
func landscapeNoticeLines() []string {
	return []string{
		i18n.T(i18n.KeyOrientationTitle),
		i18n.T(i18n.KeyOrientationBody),
	}
}

// drawLandscapeUnsupported paints the full-screen rotate-to-portrait notice. It
// replaces the normal grid/drum panes entirely while the mobile layout is in
// landscape.
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
