package ui

// synth_panel_caption.go — extracted from synth_panel_zone.go in
// Phase 2 of the audio-panel design-system compliance pass. Carries
// the small set of caption-trimming helpers the synth tab uses so the
// per-file sizing-literal ratchet (Phase 1) can target synth_panel
// _zone.go without dragging caption code along for the ride.

// truncCaptionMeasured fits text to maxPxW using measure — which MUST be
// the same metric the caller draws the text with (body TextWidth for
// DrawTextAt/AtScale callers, StyledTextWidth(·, role) for DrawTextStyled
// callers). Prefix-measured, never per-rune summed: with a proportional
// font, summing isolated rune widths loses kerning and side bearings so
// the error accumulates and truncation drifts (the same failure class as
// the fuzzy-highlight misalignment — see drawTextHighlights).
func truncCaptionMeasured(text string, maxPxW int, measure func(string) int) string {
	if text == "" || maxPxW <= 0 {
		return ""
	}
	const ellipsis = "…"
	if measure(text) <= maxPxW {
		return text
	}
	runes := []rune(text)
	// Binary-search the longest prefix whose width (with the ellipsis
	// appended) fits. Measures whole prefixes so proportional-font
	// kerning/bearings are always accounted for.
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if measure(string(runes[:mid])+ellipsis) <= maxPxW {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo <= 0 {
		return ellipsis
	}
	return string(runes[:lo]) + ellipsis
}

// truncCaption returns text fitted to maxPxW pixels at the body font
// scale, appending an ellipsis when the input is too wide. Returns
// the original string unchanged when it already fits. Returns "…"
// when even an ellipsis-only string can't fit (i.e. maxPxW < width
// of a single ellipsis glyph).
//
// The synth tab uses this for knob captions like "tone  +0.12" which
// can overflow narrow knob cells on mobile portrait — pre-Phase-2
// the renderer just clipped at the edge, producing unreadable
// half-glyphs. truncCaption keeps the caption legible by trading
// trailing characters for an explicit ellipsis.
func truncCaption(text string, maxPxW int) string {
	return truncCaptionMeasured(text, maxPxW, TextWidth)
}

// truncCaptionStyled is truncCaption for text drawn via DrawTextStyled:
// it measures with the styled role metric. Required whenever the caller
// renders at a TextRole — the body TextWidth under-measures the larger
// SemiBold roles, so body-metric truncation overflows the target rect
// (the synth-header caption ran under the Preview button this way).
func truncCaptionStyled(text string, maxPxW int, role TextRole) string {
	return truncCaptionMeasured(text, maxPxW, func(s string) int {
		return StyledTextWidth(s, role)
	})
}
