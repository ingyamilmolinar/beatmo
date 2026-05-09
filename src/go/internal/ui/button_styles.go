package ui

// RowKebabChipStyle is the visual style for the per-row context-menu kebab
// button. It renders as a rounded.sm chip with a faint on-surface fill at
// AlphaSubtle — visible enough to hint at tappability without competing with
// the row-label text. Addresses critique B7.
var RowKebabChipStyle = ButtonStyle{
	Fill:   WithAlpha(genColorOnSurface, genAlphaSubtle),
	Border: WithAlpha(genColorOnSurface, genAlphaSubtle),
	Radius: RadiusSM,
}

// PrimaryActionStyle is the style for the dominant call-to-action button on a
// surface — currently the mobile transport's play button while playback is
// stopped. It uses the accent fill so the eye is drawn to the next action;
// once playback starts and play flips to pause, the caller is expected to
// swap back to the recede style (TransportPlayStyle) so the playing state
// doesn't shout.
//
// The style mirrors ComponentButtonSecondary's chrome (radius, height,
// interaction deltas) but replaces the fill with colAccent and the border
// with the same accent so the button reads as a single saturated chip.
var PrimaryActionStyle = primaryActionStyle()

func primaryActionStyle() ButtonStyle {
	base := ButtonStyleFromSpec(ComponentButtonSecondary)
	base.Fill = colAccent
	// Match the border to the accent fill so the button reads as a single
	// saturated chip rather than a bordered control. WithAlpha returns NRGBA
	// at full opacity, matching the type of base.Border from the spec.
	base.Border = WithAlpha(colAccent, 255)
	return base
}
