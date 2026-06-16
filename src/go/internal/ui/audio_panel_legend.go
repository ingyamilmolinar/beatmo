package ui

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// audio_panel_legend.go — kid-friendly explanation chip + popover for
// the audio panel (Phase 5 of the redesign).
//
// Every tab gets a "?" pill in the sticky bar. Clicking it toggles a
// 220-px-wide popover anchored beneath the bar with a 3-sentence
// plain-English explanation. The popover is drawn directly into the
// panel rect by drawAudioPanelLegend; the parent zone owns the
// open/closed state.
//
// The text content is sourced from audioPanelLegendText[tab] so
// adding a new tab only requires adding one map entry.

// audioPanelLegendText maps each PanelTab to the i18n key for its
// short, kid-readable explanation of what the tab shows. Each line
// ≤ ~120 chars so the 220-px popover wraps to ≤ 3 lines at body font.
var audioPanelLegendText = map[PanelTab]i18n.Key{
	TabWave:     i18n.KeyLegendWave,
	TabSpectrum: i18n.KeyLegendSpectrum,
	TabMeters:   i18n.KeyLegendMeters,
	TabEQ:       i18n.KeyLegendEQ,
	TabScope:    i18n.KeyLegendScope,
	TabSynth:    i18n.KeyLegendSynth,
	TabSampler:  i18n.KeyLegendSampler,
}

// LegendText returns the kid-readable explanation for the supplied
// tab in the active locale, or "" for unknown tabs.
func LegendText(tab PanelTab) string {
	k, ok := audioPanelLegendText[tab]
	if !ok {
		return ""
	}
	return i18n.T(k)
}

// drawAudioPanelLegend paints the popover sheet for the currently-
// active tab anchored at chipR (the "?" pill's screen-space rect).
// Returns the popover's rectangle so the parent can register a
// dismiss hit-area covering the rest of the screen.
//
// The popover sits below the chip (typical "info bubble" placement).
// When the chip is too close to the right edge for a 220-px sheet
// to fit, the sheet's right edge clamps to panelMaxX-4 so the box
// stays fully visible.
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
	// Wrap the text to lines that fit sheetW - 2*padX, at body scale.
	maxTextW := sheetW - 2*padX
	lines := wrapTextToWidth(text, maxTextW)
	lineH := TextHeight() + lineGap
	sheetH := padY*2 + lineH*len(lines)

	// Position: below the chip, right-aligned to it when possible so
	// the visual association is obvious. Clamp to panelMaxX.
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
		DrawTextColorAt(dst, ln, x0+padX, ty, TokenTextPrimary())
		ty += lineH
	}
	return sheet
}

// wrapTextToWidth breaks `s` into lines that each fit within
// maxPixelW at the body font. Word-aware — never splits a word. Only
// uses spaces as the break point (matches the original kid-readable
// sentences which don't contain hyphenated long words).
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
		if TextWidth(candidate) <= maxPixelW {
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
