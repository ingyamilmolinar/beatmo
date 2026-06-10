package ui

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
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

// audioPanelLegendText maps each PanelTab to a short, kid-readable
// explanation of what the tab shows. Each line ≤ ~120 chars so the
// 220-px popover wraps to ≤ 3 lines at body font.
var audioPanelLegendText = map[PanelTab]string{
	TabWave: "Wave shows the live sound as a moving line. Tall bumps mean loud beats; flat means silence. " +
		"Use the cursor to find what made the noise.",
	TabSpectrum: "Spectrum shows which notes are loud. Bass on the left, treble on the right. " +
		"The slope button tilts the picture so pink noise looks flat — kid-friendly tonal balance.",
	TabMeters: "Levels shows how loud each instrument is. Green is safe, yellow is loud, red is too loud. " +
		"The big HEADROOM number tells you how much room is left before clipping.",
	TabEQ: "EQ shapes the sound's tone. Bend the curve up to boost a frequency, down to cut. " +
		"Mute a band with M to hear what it was adding.",
	TabScope: "Chain shows the signal flowing through six stages. Click a stage to compare it (A) " +
		"against another (B). Overlay/Split/Diff change how A and B are drawn against each other.",
	TabSynth: "Synth lets you reshape each instrument's voice. Spin a knob and the next beat plays the new sound. " +
		"OUT shows what reaches the master.",
	TabSampler: "Sampler turns a sound into your own instrument. Grab a WAV or capture the current synth, " +
		"drag the handles to trim it, tune it up or down, then Save to add it to your instruments.",
}

// LegendText returns the kid-readable explanation for the supplied
// tab, or "" for unknown tabs.
func LegendText(tab PanelTab) string { return audioPanelLegendText[tab] }

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
	drawRoundedRect(dst, sheet, TokenSurface2(), 6, true)
	drawRoundedRect(dst, sheet, TokenAccent(), 6, false)
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
