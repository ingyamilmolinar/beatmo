package ui

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// A new tab needs only an entry in audioPanelLegendText.

// At most ~120 chars, so the popover wraps to three lines.
var audioPanelLegendText = map[PanelTab]i18n.Key{
	TabWave:     i18n.KeyLegendWave,
	TabSpectrum: i18n.KeyLegendSpectrum,
	TabMeters:   i18n.KeyLegendMeters,
	TabEQ:       i18n.KeyLegendEQ,
	TabScope:    i18n.KeyLegendScope,
	TabSynth:    i18n.KeyLegendSynth,
	TabSampler:  i18n.KeyLegendSampler,
}

// "" for unknown tabs.
func LegendText(tab PanelTab) string {
	k, ok := audioPanelLegendText[tab]
	if !ok {
		return ""
	}
	return i18n.T(k)
}

// Returns the popover rect so the parent can register a dismiss area; clamped to stay visible.
func drawAudioPanelLegend(dst *ebiten.Image, chipR image.Rectangle, panelMaxX int, tab PanelTab) image.Rectangle {
	text := LegendText(tab)
	if text == "" || chipR.Empty() {
		return image.Rectangle{}
	}
	const (
		sheetW  = 220
		padX    = 8
		padY    = 6
		lineGap = 2
	)
	// Wrap the text to lines that fit sheetW - 2*padX, at caption role.
	maxTextW := sheetW - 2*padX
	lines := wrapTextToWidth(text, maxTextW)
	lineH := StyledTextHeight(RoleCaption) + lineGap
	sheetH := padY*2 + lineH*len(lines)

	x1 := chipR.Max.X
	if x1 > panelMaxX-4 {
		x1 = panelMaxX - 4
	}
	x0 := x1 - sheetW
	if x0 < chipR.Min.X-sheetW/2 {
		x0 = chipR.Min.X
		x1 = x0 + sheetW
		if x1 > panelMaxX-4 {
			x1 = panelMaxX - 4
			x0 = x1 - sheetW
		}
	}
	y0 := chipR.Max.Y + 4
	y1 := y0 + sheetH
	sheet := image.Rect(x0, y0, x1, y1)
	drawRoundedRect(dst, sheet, TokenSurface2(), RadiusXS, true)
	drawRoundedRect(dst, sheet, TokenAccent(), RadiusXS, false)
	ty := y0 + padY
	for _, ln := range lines {
		DrawTextStyled(dst, ln, x0+padX, ty, RoleCaption, TokenTextPrimary())
		ty += lineH
	}
	return sheet
}

// Never splits a word; spaces are the only break points.
func wrapTextToWidth(s string, maxPixelW int) []string {
	if maxPixelW <= 0 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{}
	}
	out := []string{}
	cur := ""
	for _, w := range words {
		candidate := cur
		if candidate == "" {
			candidate = w
		} else {
			candidate = cur + " " + w
		}
		if StyledTextWidth(candidate, RoleCaption) <= maxPixelW {
			cur = candidate
			continue
		}
		if cur != "" {
			out = append(out, cur)
		}
		cur = w
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
