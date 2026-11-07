//go:build test

package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Real builds measure INK bounds, which are not additive (kerning, side bearings, a space measures 0); the
// monospace test font is additive, which hides misplaced highlights from every other test.
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

// Body and styled metrics both, so label layout and highlight layout see the same font, as in production.
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

// The label is drawn with the styled proportional font, so cell i must span [measure(label[:i]),
// measure(label[:i+1])] from labelX; summing isolated runes drifts left of the glyphs. A space gets no cell.
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

	// A label with a space is the worst case for per-rune summation.
	target, label := "", ""
	for _, id := range comp.state.filteredInsts {
		l := comp.state.displayLabelByID[id]
		if l == "" {
			l = string(id)
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
			target, label = string(id), l
		}
	}
	if target == "" {
		t.Fatal("no instrument with a multi-character label found")
	}

	// Type the full label: the exact-substring fast path highlights every rune.
	feedNativeSearch(t, g, strings.ToLower(label))
	if got := comp.state.searchHighlights[instrument.ID(target)]; len(got) != len([]rune(label)) {
		t.Fatalf("expected every rune of %q highlighted, got positions %v", label, got)
	}

	// Locate the target's visible row button.
	var btn *Button
	for i, id := range comp.instIDs {
		if id == instrument.ID(target) && i < len(comp.instBtns) {
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

	// Mirrors drawMenuRow's swatch-row label placement.
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
