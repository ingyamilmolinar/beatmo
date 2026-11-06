package ui

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
var PrimaryActionStyle ButtonStyle // derived in rebuildThemeAliases (theme.go) via primaryActionStyle()

func primaryActionStyle() ButtonStyle {
	base := ButtonStyleFromSpec(ComponentButtonSecondary).withLive(liveFillAccent)
	// Match the border to the accent fill so the button reads as a single
	// saturated chip rather than a bordered control. WithAlpha returns NRGBA
	// at full opacity, matching the type of base.Border from the spec.
	base.Border = WithAlpha(colAccent, 255)
	return base
}
