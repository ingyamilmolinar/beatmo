//go:build test

package ui

import (
	"image"
	"strings"
	"testing"
)

// installDivergentRoleMetrics swaps the font hooks for fakes where the
// styled role metric out-measures the body metric (as in production: the
// 16px SemiBold RoleSectionHeader is wider per glyph than the body font
// TextWidth). Under -tags test the debug font makes the two metrics
// identical, which masks exactly the class of bug this file pins — see the
// metric-fake recipe in inst_menu_search_highlight_render_test.go.
func installDivergentRoleMetrics(t *testing.T) {
	t.Helper()
	origW, origH, origStyled := textMeasureWidth, textMeasureHeight, styledTextMeasure
	textMeasureWidth = func(s string) int { return 6 * len([]rune(s)) }
	textMeasureHeight = func() int { return 13 }
	styledTextMeasure = func(s string, size float64, bold bool) (int, int) {
		return int(float64(len([]rune(s))) * size * 0.6), int(size)
	}
	t.Cleanup(func() {
		textMeasureWidth, textMeasureHeight, styledTextMeasure = origW, origH, origStyled
	})
}

// The header caption is DRAWN at RoleSectionHeader (16px SemiBold) but was
// truncated against the body-font TextWidth metric — an under-measure, so
// long "inst — recipe" captions overflowed the caption rect into the
// Preview button (desktop `eq_tab_synth` screenshot: "synth-modular-kick"
// colliding with Preview; A7 in the 2026-07-04 critique). The truncation
// must use the same styled metric the draw call uses.
func TestSynthHeaderCaptionFitsItsRect(t *testing.T) {
	installDivergentRoleMetrics(t)

	h := synthHeaderLayout{
		instLabel:   "dnb-kick",
		recipeID:    "synth-modular-kick-dnb-very-long-recipe-name",
		captionRect: image.Rect(0, 0, 180, 24),
	}
	got := synthHeaderCaptionText(h)
	if w := StyledTextWidth(got, RoleSectionHeader); w > h.captionRect.Dx() {
		t.Fatalf("caption %q measures %dpx at RoleSectionHeader, wider than its %dpx rect", got, w, h.captionRect.Dx())
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("long caption %q not ellipsized", got)
	}
}

// A caption that fits is passed through untouched.
func TestSynthHeaderCaptionShortPassesThrough(t *testing.T) {
	installDivergentRoleMetrics(t)

	h := synthHeaderLayout{
		instLabel:   "kick",
		captionRect: image.Rect(0, 0, 400, 24),
	}
	if got := synthHeaderCaptionText(h); got != "kick" {
		t.Fatalf("short caption altered: %q", got)
	}
}
