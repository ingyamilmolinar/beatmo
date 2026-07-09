//go:build test

package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// proportionalInkMeasure mimics how real (non-test) builds measure text:
// text.BoundString(face, s).Dx() returns the INK bounds of the whole string —
// per-glyph advances plus kerning, minus the side bearings, and a trailing
// space contributes no ink at all. The critical property is that the measure
// is NOT additive: measure("ab") != measure("a") + measure("b"), and
// measure(" ") == 0. The test-build debug font is monospace (additive), which
// is exactly why the production misalignment is invisible to every other test.
func proportionalInkAdvance(r rune) int {
	if r == ' ' {
		return 5
	}
	return 4 + int(r%4) // 4..7 px — varied per char like a proportional face
}

func proportionalInkMeasure(s string) int {
	rs := []rune(s)
	end := len(rs)
	for end > 0 && rs[end-1] == ' ' {
		end-- // trailing space has no ink, like text.BoundString
	}
	if end == 0 {
		return 0
	}
	w := 0
	for _, r := range rs[:end] {
		w += proportionalInkAdvance(r)
	}
	return w - 2 // side bearings: ink is narrower than the advance sum
}

// installProportionalFontMetrics swaps the package font-measurement hooks for
// the proportional fake above, for both the body metric (TextWidth/TextHeight)
// and the styled role metric (StyledTextWidth/StyledTextHeight), so label
// layout and highlight layout see the same "font" — as in production builds.
func installProportionalFontMetrics(t *testing.T) {
	t.Helper()
	origW, origH, origStyled := textMeasureWidth, textMeasureHeight, styledTextMeasure
	textMeasureWidth = proportionalInkMeasure
	textMeasureHeight = func() int { return 13 }
	styledTextMeasure = func(s string, size float64, bold bool) (int, int) {
		return proportionalInkMeasure(s), 18
	}
	t.Cleanup(func() {
		textMeasureWidth, textMeasureHeight, styledTextMeasure = origW, origH, origStyled
	})
}

// TestInstMenuSearchHighlightCellsAlignWithGlyphs reproduces the reported
// mobile (and desktop) bug: the fuzzy-search character highlights in the
// instrument menu do not sit under the matched characters. drawMenuRow draws
// the label with the styled proportional font, but the highlight cells are
// positioned by summing the width of each rune measured IN ISOLATION. With a
// proportional font that summation is wrong three ways: a lone space measures
// 0 (every highlight after a space shifts a full space-width left), isolated
// glyph ink is narrower than its advance (error accumulates per character),
// and kerning is lost. The cells drift left of the glyphs they should mark.
//
// Contract asserted: the highlight cell for rune i must span exactly
// [labelX + measure(label[:i]), labelX + measure(label[:i+1])] at the label's
// styled line box — i.e. the cells tile with the rendered text, using the
// same measurement the label is drawn with. Runes with no ink (spaces) get no
// cell.
func TestInstMenuSearchHighlightCellsAlignWithGlyphs(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	resetTouchOverride()
	t.Cleanup(resetTouchOverride)
	testMobileInputValue = map[string]string{}
	t.Cleanup(func() { testMobileInputValue = nil })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	comp := navigateToInstrumentsMode(t, g)
	if len(comp.state.filteredInsts) == 0 {
		t.Fatal("no instruments available in instruments mode")
	}

	// Pick a target label — prefer one containing a space (the worst case for
	// the per-rune summation), falling back to the longest label available.
	target, label := "", ""
	for _, id := range comp.state.filteredInsts {
		l := comp.state.displayLabelByID[id]
		if l == "" {
			l = id
		}
		if len([]rune(l)) < 2 {
			continue
		}
		better := len(l) > len(label)
		if strings.ContainsRune(l, ' ') && !strings.ContainsRune(label, ' ') {
			better = true
		} else if strings.ContainsRune(label, ' ') && !strings.ContainsRune(l, ' ') {
			better = false
		}
		if target == "" || better {
			target, label = id, l
		}
	}
	if target == "" {
		t.Fatal("no instrument with a multi-character label found")
	}

	// Type the full label: the exact-substring fast path highlights every rune.
	feedNativeSearch(t, g, strings.ToLower(label))
	if got := comp.state.searchHighlights[target]; len(got) != len([]rune(label)) {
		t.Fatalf("expected every rune of %q highlighted, got positions %v", label, got)
	}

	// Locate the target's visible row button.
	var btn *Button
	for i, id := range comp.instIDs {
		if id == target && i < len(comp.instBtns) {
			btn = comp.instBtns[i]
			break
		}
	}
	if btn == nil {
		t.Fatalf("target instrument %q not among visible rows %v", target, comp.instIDs)
	}
	if len(btn.Highlights) == 0 {
		t.Fatalf("row button for %q carries no highlights", target)
	}

	// From here on, measure text like a real proportional font.
	installProportionalFontMetrics(t)

	// Capture every fuzzy-highlight rect drawn inside the target row.
	origDrawRect := drawRect
	var got []image.Rectangle
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled && sameRGBA(c, colFuzzyHighlight) && !r.Empty() && r.Overlaps(btn.Rect()) {
			got = append(got, r)
		}
		origDrawRect(dst, r, c, filled)
	}
	t.Cleanup(func() { drawRect = origDrawRect })

	screen := ebiten.NewImage(390, 844)
	comp.Draw(screen)

	// Expected cells: prefix-measured spans at the styled label line box,
	// mirroring drawMenuRow's swatch-row label placement.
	r := btn.Rect()
	labelX := r.Min.X + SpaceSM + instMenuSwatchSz + SpaceSM
	th := StyledTextHeight(RoleBody)
	ty := r.Min.Y + (r.Dy()-th)/2
	rs := []rune(btn.Text)
	var want []image.Rectangle
	for _, hi := range btn.Highlights {
		x0 := proportionalInkMeasure(string(rs[:hi]))
		x1 := proportionalInkMeasure(string(rs[:hi+1]))
		if x1 <= x0 {
			continue // rune has no ink (space) — nothing to highlight
		}
		want = append(want, image.Rect(labelX+x0, ty, labelX+x1, ty+th))
	}
	if len(want) == 0 {
		t.Fatal("test setup broken: no expected highlight cells")
	}

	if len(got) != len(want) {
		t.Fatalf("HIGHLIGHT RENDER BUG: %d highlight cells drawn for label %q, want %d.\n got: %v\nwant: %v",
			len(got), btn.Text, len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("HIGHLIGHT RENDER BUG: cell %d for label %q drawn at %v, want %v "+
				"(cells must tile with the drawn glyphs: per-rune isolated-width summation "+
				"drifts left of the proportional-font text).\n got: %v\nwant: %v",
				i, btn.Text, got[i], want[i], got, want)
		}
	}
}
