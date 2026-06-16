package ui

import "image/color"

// row_instrument_shades.go derives the per-row mute / solo / FX / kebab control
// chrome from the row's *instrument color* so every control reads as a shade of
// the hue it belongs to, instead of a fixed light-blue that drifts from the
// instrument. The shade scheme (user-approved):
//
//	mute ON  = instrument color darkened (deeper shade — "muffled")
//	solo ON  = instrument color brightened (lighter shade — "spotlight")
//	fx   ON  = the full instrument hue (+ recolored badge)
//	any OFF  = a dim low-alpha tint of the hue + a thin hued border, so each
//	           row stays "owned" by its color even when idle.
//
// The M / S / FX letters still carry the semantic meaning; lightness separates
// the three active states. All colors are derived via adjustColor / WithAlpha
// helpers — no raw color literals (keeps token_discipline_test.go clean).

// rowToggleRole identifies which per-row toggle a shaded visual is for.
type rowToggleRole int

const (
	roleMute rowToggleRole = iota
	roleSolo
	roleFX
	roleKebab
)

// Shade deltas applied to the instrument base color. Tuned so the active states
// are clearly distinct from each other and from the base hue while staying
// recognisably the same color family.
const (
	rowShadeMuteDarken  = 64 // mute active = base darkened by this much per channel
	rowShadeSoloLighten = 64 // solo active = base brightened by this much per channel
	rowShadeBorderLift  = 40 // active border lifted above the fill for edge definition
)

// rowToggleBaseRGBA resolves an arbitrary instrument color (possibly nil) to a
// concrete RGBA, falling back to the rack color fallback for headless/test rows.
func rowToggleBaseRGBA(base color.Color) color.RGBA {
	if base == nil {
		return genColorRowRackColorFallback
	}
	r, g, b, _ := base.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
}

// rowToggleFillRGBA returns the fill color for a row toggle in the given role
// and on/off state, as a shade of the instrument base color.
func rowToggleFillRGBA(base color.Color, role rowToggleRole, on bool) color.RGBA {
	b := rowToggleBaseRGBA(base)
	if !on {
		// Dim tint: a low-alpha wash of the hue reads as "off" while keeping
		// the row's identity. The kebab is dimmer still (it's always "off").
		return rowShadeToRGBA(WithAlphaFromColor(b, AlphaSubtle))
	}
	switch role {
	case roleMute:
		return rowShadeToRGBA(adjustColor(b, -rowShadeMuteDarken))
	case roleSolo:
		return rowShadeToRGBA(adjustColor(b, +rowShadeSoloLighten))
	case roleKebab:
		// The kebab has no "on" state in practice; treat as a dim tint.
		return rowShadeToRGBA(WithAlphaFromColor(b, AlphaSubtle))
	default: // roleFX
		return b
	}
}

// rowToggleStyle returns the full ButtonStyle for a row toggle, deriving fill
// and border from the instrument color. Inactive toggles get a thin hued border
// so the row stays owned by its hue; active toggles get a brighter hued edge.
func rowToggleStyle(base color.Color, role rowToggleRole, on bool) ButtonStyle {
	b := rowToggleBaseRGBA(base)
	fill := rowToggleFillRGBA(base, role, on)
	var border color.Color
	if on {
		// A slightly brighter hued edge gives the filled chip definition.
		border = rowShadeToRGBA(adjustColor(fill, +rowShadeBorderLift))
	} else {
		border = WithAlphaFromColor(b, AlphaMedium)
	}
	return ButtonStyle{Fill: fill, Border: border, Radius: RadiusSM}
}

// rowKebabStyle returns the per-row ellipsis (kebab) chip style as a dim tint of
// the instrument color, replacing the fixed RowKebabChipStyle so the overflow
// affordance matches the instrument it belongs to.
func rowKebabStyle(base color.Color) ButtonStyle {
	b := rowToggleBaseRGBA(base)
	return ButtonStyle{
		Fill:   WithAlphaFromColor(b, AlphaSubtle),
		Border: WithAlphaFromColor(b, AlphaMedium),
		Radius: RadiusSM,
	}
}

// rowShadeToRGBA narrows a color.Color (as returned by adjustColor /
// WithAlphaFromColor) to a concrete premultiplied RGBA.
func rowShadeToRGBA(c color.Color) color.RGBA {
	return color.RGBAModel.Convert(c).(color.RGBA)
}
