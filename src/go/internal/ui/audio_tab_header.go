package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Audio-tab header chrome shared by the Synth and Sampler tabs: one title
// treatment and one action-button size, so the two headers read as a family
// and sit at the same scale as the sticky bar's tab pills above them.
//
// The title is a GROUP label + the instrument: the tab name ("SYNTH") in the
// uppercase muted caption role (DESIGN.md "menu-group-header" — a category,
// not a heading) followed by the instrument's display name at body. Both sit
// on one baseline. Actions are body-role keycaps sized by
// densities.synthHeaderButtonH.

// audioTabHeaderTitleGap separates the tab label from the instrument name.
const audioTabHeaderTitleGap = SpaceSM

// audioTabActionButtonPadX is the horizontal inset of a header action label.
const audioTabActionButtonPadX = SpaceMD

// audioTabActionButtonWidth is the width of a text action keycap in an audio
// tab (Preview / Save / Save As / Reset, and the Sampler's Rev / Norm / Fade
// row): the body-role label plus a fixed inset.
func audioTabActionButtonWidth(label string) int {
	return StyledTextWidth(label, RoleBody) + 2*audioTabActionButtonPadX
}

// drawAudioTabHeaderTitle draws "<TAB> <name>" left-aligned at x inside the
// header band r, clipped to maxW. name may be empty (tab label only).
func drawAudioTabHeaderTitle(dst *ebiten.Image, r image.Rectangle, x, maxW int, tab, name string) {
	if maxW <= 0 || r.Empty() {
		return
	}
	bodyH := StyledTextHeight(RoleBody)
	capH := StyledTextHeight(RoleCaption)
	// Shared baseline: the caption's line box bottom-aligns with the body's.
	bodyY := r.Min.Y + (r.Dy()-bodyH)/2
	capY := bodyY + bodyH - capH

	label := truncCaptionStyled(tab, maxW, RoleCaption)
	DrawTextStyled(dst, label, x, capY, RoleCaption, TokenTextSecondary())
	if name == "" {
		return
	}
	nx := x + StyledTextWidth(label, RoleCaption) + audioTabHeaderTitleGap
	if rest := maxW - (nx - x); rest > 0 {
		DrawTextStyled(dst, truncCaptionStyled(name, rest, RoleBody), nx, bodyY, RoleBody, TokenTextPrimary())
	}
}

// audioTabHeaderName is the instrument half of the header title — its display
// name, else its id; "" when no instrument is selected. The recipe id is never
// shown when a name exists — it is an engine identifier, not something a user
// recognises.
func audioTabHeaderName(instID instrument.ID) string {
	if name := audio.InstrumentDisplayName(instID); name != "" {
		return name
	}
	return string(instID)
}
