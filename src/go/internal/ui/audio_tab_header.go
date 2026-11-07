package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// One title treatment and one button size for Synth and Sampler, at the sticky bar's scale: the tab
// name as a muted caption, then the instrument at body, on one baseline.

// audioTabHeaderTitleGap separates the tab label from the instrument name.
const audioTabHeaderTitleGap = SpaceSM

// audioTabActionButtonPadX is the horizontal inset of a header action label.
const audioTabActionButtonPadX = SpaceMD

func audioTabActionButtonWidth(label string) int {
	return StyledTextWidth(label, RoleBody) + 2*audioTabActionButtonPadX
}

// name may be empty.
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

// The display name, else the id; never the recipe id when a name exists.
func audioTabHeaderName(instID instrument.ID) string {
	if name := audio.InstrumentDisplayName(instID); name != "" {
		return name
	}
	return string(instID)
}
