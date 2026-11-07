package ui

// measure MUST be the metric the caller draws with. Whole prefixes are measured, never rune widths summed: that
// loses kerning and bearings, so the truncation drifts.
func truncCaptionMeasured(text string, maxPxW int, measure func(string) int) string {
	if text == "" || maxPxW <= 0 {
		return ""
	}
	const ellipsis = "…"
	if measure(text) <= maxPxW {
		return text
	}
	runes := []rune(text)
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

// Trades trailing characters for an explicit ellipsis rather than clipping half-glyphs; "…" when not even
// that fits.
func truncCaption(text string, maxPxW int) string {
	return truncCaptionMeasured(text, maxPxW, TextWidth)
}

// Required for TextRole text: the body metric under-measures the SemiBold roles and the truncation overflows.
func truncCaptionStyled(text string, maxPxW int, role TextRole) string {
	return truncCaptionMeasured(text, maxPxW, func(s string) int {
		return StyledTextWidth(s, role)
	})
}
