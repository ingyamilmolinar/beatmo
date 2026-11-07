package ui

// The accent-filled call to action (mobile Play while stopped); the caller swaps back to TransportPlayStyle
// once playing so the playing state does not shout.
var PrimaryActionStyle ButtonStyle // derived in rebuildThemeAliases (theme.go) via primaryActionStyle()

func primaryActionStyle() ButtonStyle {
	base := ButtonStyleFromSpec(ComponentButtonSecondary).withLive(liveFillAccent)
	// The border matches the fill so it reads as one saturated chip.
	base.Border = WithAlpha(colAccent, 255)
	return base
}
