package ui

// synth_panel_caption.go — extracted from synth_panel_zone.go in
// Phase 2 of the audio-panel design-system compliance pass. Carries
// the small set of caption-trimming helpers the synth tab uses so the
// per-file sizing-literal ratchet (Phase 1) can target synth_panel
// _zone.go without dragging caption code along for the ride.

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
	if text == "" || maxPxW <= 0 {
		return ""
	}
	const ellipsis = "…"
	if TextWidth(text) <= maxPxW {
		return text
	}
	// Reserve space for the ellipsis itself.
	ellipsisW := TextWidth(ellipsis)
	if maxPxW <= ellipsisW {
		return ellipsis
	}
	available := maxPxW - ellipsisW
	// Walk runes until the next addition would overflow. Walking by
	// rune avoids splitting a multi-byte UTF-8 codepoint mid-byte —
	// the existing TextWidth doesn't tolerate invalid UTF-8 input.
	runes := []rune(text)
	var w int
	cut := 0
	for i, r := range runes {
		next := TextWidth(string([]rune{r}))
		if w+next > available {
			cut = i
			break
		}
		w += next
		cut = i + 1
	}
	if cut <= 0 {
		return ellipsis
	}
	return string(runes[:cut]) + ellipsis
}
