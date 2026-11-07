package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Pure state → text, so the wording is testable without rendering.

// A if unset, else B if unset, else it REPLACES B: the rule stays, only now it is said. Words, not
// arrows: chrome arrow glyphs are banned.
func chainNextTapHint(tapA, tapB scope.Stage) string {
	switch {
	case tapA < 0:
		return i18n.T(i18n.KeyChainNextTapA)
	case tapB < 0:
		return i18n.T(i18n.KeyChainNextTapB)
	default:
		return i18n.T(i18n.KeyChainNextTapReplace)
	}
}

type chainLegendEntry struct {
	Slot  string // "A" or "B"
	Stage string // localized stage label, or the invitation when unset
	Color color.NRGBA
	Set   bool
}

// Both slots always: an UNSET slot is the case that needed explaining.
func chainLegend(tapA, tapB scope.Stage) []chainLegendEntry {
	return []chainLegendEntry{
		chainLegendSlot("A", tapA, colScopeA),
		chainLegendSlot("B", tapB, colScopeB),
	}
}

func chainLegendSlot(slot string, st scope.Stage, col color.NRGBA) chainLegendEntry {
	if st < 0 {
		return chainLegendEntry{Slot: slot, Stage: i18n.T(i18n.KeyChainLegendUnset), Color: col}
	}
	return chainLegendEntry{Slot: slot, Stage: chainStageLabelLocalized(st), Color: col, Set: true}
}

// The pill codes stay (they match every other analyzer pill and fit the row); this carries the meaning.
func chainModeGloss(m chainDisplayMode) string {
	switch m {
	case chainOverlay:
		return i18n.T(i18n.KeyChainGlossOverlay)
	case chainSplit:
		return i18n.T(i18n.KeyChainGlossSplit)
	case chainDiff:
		return i18n.T(i18n.KeyChainGlossDiff)
	}
	return ""
}

// So the Chain codes answer to the same app-wide lookup.
func chainPillGlosses() map[string]i18n.Key {
	return map[string]i18n.Key{
		"OVR": i18n.KeyChainGlossOverlay,
		"SPL": i18n.KeyChainGlossSplit,
		"DIF": i18n.KeyChainGlossDiff,
		"FRZ": i18n.KeyChainGlossFreeze,
	}
}

// Clips itself; draws nothing when too narrow (mobile) to avoid the pills.
func drawChainHeaderStrip(dst *ebiten.Image, strip image.Rectangle, tapA, tapB scope.Stage, mode chainDisplayMode) {
	if strip.Dx() < chainHeaderStripMinW || strip.Dy() <= 0 {
		return
	}
	x := strip.Min.X
	th := StyledTextHeight(RoleMicro)
	y := strip.Min.Y + (strip.Dy()-th)/2

	// First, next to the stage column: the column itself is too narrow for the replace warning.
	hint := chainNextTapHint(tapA, tapB)
	if hw := StyledTextWidth(hint, RoleMicro); x+hw <= strip.Max.X {
		DrawTextStyled(dst, hint, x, y, RoleMicro, WithAlpha(colTextSecondary, AlphaMedium))
		x += hw + SpaceMD
	}

	// A swatch in the trace's own colour; an unset slot still shows.
	for _, e := range chainLegend(tapA, tapB) {
		sw := chainLegendSwatchW
		swY := strip.Min.Y + (strip.Dy()-chainLegendSwatchH)/2
		col := e.Color
		if !e.Set {
			// Dim, not hidden: an unset slot must still read as a slot.
			col = WithAlphaNRGBA(e.Color, AlphaSubtle)
		}
		drawRect(dst, image.Rect(x, swY, x+sw, swY+chainLegendSwatchH), col, true)
		x += sw + SpaceXS

		txt := e.Slot + " " + e.Stage
		tc := color.NRGBA(colTextSecondary)
		if !e.Set {
			tc = WithAlpha(colTextSecondary, AlphaMedium)
		}
		if x+StyledTextWidth(txt, RoleMicro) > strip.Max.X {
			return
		}
		DrawTextStyled(dst, txt, x, y, RoleMicro, tc)
		x += StyledTextWidth(txt, RoleMicro) + SpaceMD
	}

	// Dropped first when tight: the legend is the load-bearing half.
	gloss := chainModeGloss(mode)
	if gloss == "" {
		return
	}
	if x+StyledTextWidth(gloss, RoleMicro) > strip.Max.X {
		return
	}
	DrawTextStyled(dst, gloss, x, y, RoleMicro, WithAlpha(colTextSecondary, AlphaMedium))
}

const (
	// Below this the legend would run under the mode pills.
	chainHeaderStripMinW = 180
	chainLegendSwatchW   = 10
	chainLegendSwatchH   = 3
)
