package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// OFF is the neutral row-control body (identity lives once per row, on the label swatch); mute ON
// darkens, solo ON brightens, FX ON is the full hue; the kebab is always neutral. An ON letter takes
// whichever of on-surface / background contrasts more.

// rowToggleRole identifies which per-row toggle a shaded visual is for.
type rowToggleRole int

const (
	roleMute rowToggleRole = iota
	roleSolo
	roleFX
	roleKebab
)

// Distinct from each other and the base hue while staying in its family.
const (
	rowShadeMuteDarken  = 64 // mute active = base darkened by this much per channel
	rowShadeSoloLighten = 64 // solo active = base brightened by this much per channel
)

// Letters, not icons: a slashed speaker read as muted at rest, and the FX sparkle read as "+".
const (
	rowMuteLabel = "M"
	rowSoloLabel = "S"
	rowFXLabel   = "FX"
)

// nil falls back to the rack color for headless rows.
func rowToggleBaseRGBA(base color.Color) color.RGBA {
	if base == nil {
		return genColorRowRackColorFallback
	}
	r, g, b, _ := base.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
}

// OFF (and the kebab) is the neutral body.
func rowToggleFillRGBA(base color.Color, role rowToggleRole, on bool) color.RGBA {
	if !on || role == roleKebab {
		return rowShadeToRGBA(InstButtonStyle.fill())
	}
	b := rowToggleBaseRGBA(base)
	var fill color.RGBA
	switch role {
	case roleMute:
		fill = rowShadeToRGBA(adjustColor(b, -rowShadeMuteDarken))
	case roleSolo:
		fill = rowShadeToRGBA(adjustColor(b, +rowShadeSoloLighten))
	default: // roleFX
		fill = b
	}
	return legibleToggleFill(fill)
}

const rowToggleMinContrast = 4.5

// A mid-luminance hue fits neither letter color, so deepen it slightly: same identity, readable.
func legibleToggleFill(fill color.RGBA) color.RGBA {
	for i := 0; i < 16; i++ {
		lf := relLuminance(fill)
		if wcagContrast(lf, relLuminance(legibleOn(fill))) >= rowToggleMinContrast {
			return fill
		}
		fill = rowShadeToRGBA(adjustColor(fill, -6))
	}
	return fill
}

// A latched toggle draws at rest (pressed is a STATE flag here), so the press dim never stacks on the
// shade. The neutral body is InstButtonStyle with its live spec tag, so it re-themes.
func rowToggleStyle(base color.Color, role rowToggleRole, on bool) rowToggleVisual {
	if !on || role == roleKebab {
		st := InstButtonStyle
		st.Radius = RadiusControl
		return rowToggleVisual{st}
	}
	return rowToggleVisual{ButtonStyle{Fill: rowToggleFillRGBA(base, role, on), Radius: RadiusControl}}
}

// rowToggleVisual is a ButtonStyle whose body ignores the pressed flag.
type rowToggleVisual struct{ ButtonStyle }

func (v rowToggleVisual) Draw(dst *ebiten.Image, r image.Rectangle, _, hovered bool) {
	v.ButtonStyle.Draw(dst, r, false, hovered)
}

func rowToggleLabelColor(base color.Color, role rowToggleRole, on bool) color.Color {
	if !on || role == roleKebab {
		return colTextSecondary
	}
	return legibleOn(rowToggleFillRGBA(base, role, on))
}

// The pressed STATE flag is what the controls cache compares.
func applyRowToggle(btn *Button, base color.Color, role rowToggleRole, on bool) {
	if btn == nil {
		return
	}
	btn.Style = rowToggleStyle(base, role, on)
	col := rowToggleLabelColor(base, role, on)
	btn.TextColor = col
	btn.IconColor = col
	btn.pressed = on
}

// No state, so no instrument color.
func rowKebabStyle(base color.Color) rowToggleVisual {
	return rowToggleStyle(base, roleKebab, false)
}

func rowShadeToRGBA(c color.Color) color.RGBA {
	return color.RGBAModel.Convert(c).(color.RGBA)
}

// Whichever has the higher WCAG contrast against fill.
func legibleOn(fill color.Color) color.Color {
	lf := relLuminance(fill)
	light := wcagContrast(lf, relLuminance(colTextPrimary))
	dark := wcagContrast(lf, relLuminance(genColorBackground))
	if dark > light {
		return genColorBackground
	}
	return colTextPrimary
}

// relLuminance is the WCAG relative luminance of an opaque color.
func relLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		s := float64(v) / 0xffff
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// wcagContrast is the WCAG contrast ratio of two relative luminances.
func wcagContrast(a, b float64) float64 {
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}
