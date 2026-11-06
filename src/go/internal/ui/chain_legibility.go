package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Chain-tab legibility helpers.
//
// The Chain tab's model is unchanged — two taps (A and B) on the six pipeline
// stages, three display modes, auto-fit, auto-gain and freeze. What was missing
// was any on-screen account of it: nothing said which trace colour was which,
// nothing said that B existed, and nothing said what the next tap would do.
//
// These three helpers are pure state->value functions so the WORDING is
// testable without rendering. The header strip that draws them was previously
// empty from the stage column across to the zoom readout, so all of this costs
// no layout.

// chainNextTapHint describes what the NEXT stage tap will do.
//
// handleStageClick assigns A if A is unset, else B if B is unset, else it
// REPLACES B. That rule is deliberately unchanged here — it is the hidden-ness
// that made it hard to use, not the rule. The replace case is the one worth
// warning about: with both taps set, every further tap silently discarded B.
//
// Worded, never arrowed: DESIGN.md bans raw chrome arrow glyphs in UI labels.
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

// chainLegendEntry is one row of the A/B legend: a swatch in the trace's own
// colour, the slot it belongs to, and the stage it is tapping.
type chainLegendEntry struct {
	Slot  string // "A" or "B"
	Stage string // localized stage label, or the invitation when unset
	Color color.NRGBA
	Set   bool
}

// chainLegend returns both slots, always — an UNSET slot is exactly the case
// that needed explaining, since a user with only A assigned had nothing telling
// them a second trace was even possible.
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

// chainModeGloss returns the plain-English meaning of the active display mode.
//
// Chain was the only audio-panel tab that never used the PlainEnglish
// convention Synth, EQ and Sampler all rely on, so OVR / SPL / DIF sat on
// screen as three unexplained codes. The codes stay — they match every other
// analyzer pill (AG / FIT / K20 / CLR / RST) and spelling them out would not
// fit the pill row — and this gloss carries the meaning instead.
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

// chainPillGlosses feeds PlainEnglish so the Chain codes answer to the same
// app-wide lookup every other tab's labels do.
func chainPillGlosses() map[string]i18n.Key {
	return map[string]i18n.Key{
		"OVR": i18n.KeyChainGlossOverlay,
		"SPL": i18n.KeyChainGlossSplit,
		"DIF": i18n.KeyChainGlossDiff,
		"FRZ": i18n.KeyChainGlossFreeze,
	}
}

// drawChainHeaderStrip paints the A/B legend and the active mode's gloss into
// the header band, between the stage column and the zoom readout.
//
// That span was entirely EMPTY before this — the only thing in the header row
// was the zoom readout pinned to the far right — so the whole explanatory layer
// costs no layout and steals nothing from the trace.
//
// Returns nothing; it clips itself to the space actually available and draws
// nothing at all when the strip is too narrow (mobile), where the legend would
// collide with the pills.
func drawChainHeaderStrip(dst *ebiten.Image, strip image.Rectangle, tapA, tapB scope.Stage, mode chainDisplayMode) {
	if strip.Dx() < chainHeaderStripMinW || strip.Dy() <= 0 {
		return
	}
	x := strip.Min.X
	th := StyledTextHeight(RoleMicro)
	y := strip.Min.Y + (strip.Dy()-th)/2

	// Next-tap hint FIRST, hard against the stage column it describes. It
	// cannot live inside the column itself: chainStageColW is 48-72px and
	// "NEXT TAP REPLACES B" does not fit, so a column-hosted hint vanished in
	// precisely the case that needed it — both taps set, next tap discards B.
	hint := chainNextTapHint(tapA, tapB)
	if hw := StyledTextWidth(hint, RoleMicro); x+hw <= strip.Max.X {
		DrawTextStyled(dst, hint, x, y, RoleMicro, WithAlpha(colTextSecondary, AlphaMedium))
		x += hw + SpaceMD
	}

	// Legend: a swatch in the trace's OWN colour, then "A Synth" / "B EQ". An
	// unset slot still appears, reading "B tap a stage" — a user with only A
	// assigned previously had nothing telling them a second trace existed.
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

	// Active-mode gloss, dropped first when the strip is tight — the legend is
	// the load-bearing half.
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
	// chainHeaderStripMinW is the narrowest strip worth drawing into. Below
	// this the legend would run under the mode pills, so the strip stays empty
	// (mobile, where the pills move to their own row).
	chainHeaderStripMinW = 180
	chainLegendSwatchW   = 10
	chainLegendSwatchH   = 3
)
