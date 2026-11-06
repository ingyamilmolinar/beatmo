package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// row_instrument_shades.go derives the per-row mute / solo / FX / kebab control
// chrome. The scheme (DESIGN.md "Per-instrument control shading"):
//
//	any OFF  = the neutral row-control body (the same surface-2 keycap every
//	           other rack control wears) with a muted letter — identity at rest
//	           lives ONCE per row, in the label's leading swatch, so 28 tinted
//	           idle chips no longer compete with the state that matters
//	mute ON  = instrument color darkened (deeper shade — "muffled")
//	solo ON  = instrument color brightened (lighter shade — "spotlight")
//	fx   ON  = the full instrument hue (+ recolored badge)
//	kebab    = always neutral (it has no state)
//
// An ON chip's letter is whichever of on-surface / background contrasts more
// with its fill, so a pale instrument (yellow, mint) gets a dark letter and a
// deep one (blue, magenta) a light letter — the two safe contrast patterns.
// The M / S / FX letters carry the meaning; lightness separates the three
// active states. All shades derive via adjustColor — no raw color literals.

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
)

// Row-toggle letters. Single-character / two-letter text labels are the
// sanctioned row-control glyphs (DESIGN.md "Semantic IconColor mapping"); a
// slashed-speaker icon read as "muted" even at rest, and the FX sparkle read
// as a "+" at chip size, next to the add-row "+".
const (
	rowMuteLabel = "M"
	rowSoloLabel = "S"
	rowFXLabel   = "FX"
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
// and on/off state. OFF (and the kebab) is the neutral row-control body.
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

// rowToggleMinContrast is the DESIGN.md text contrast target an active
// toggle's letter must clear on its fill.
const rowToggleMinContrast = 4.5

// legibleToggleFill darkens an active fill in small steps until its best
// letter color (legibleOn) clears rowToggleMinContrast. A mid-luminance hue
// (orchid, electric blue) is too light for a light letter and too dark for a
// dark one at full strength; a slightly deeper shade of the same hue keeps
// the identity and makes the letter readable. Most fills pass unchanged.
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

// rowToggleStyle returns the body visual for a row toggle. A latched toggle
// is drawn at rest (pressed is a STATE flag on these buttons — the rack syncs
// it to Muted/Solo/active-FX for cache invalidation — not a finger on the
// key), so the -20 press dim never stacks on the active shade.
//
// The neutral (off / kebab) body is InstButtonStyle itself, carrying its live
// spec tag, so it re-resolves under a runtime theme exactly like the label and
// volume bodies beside it. Capturing the fill as a plain color froze the
// kebab — whose style is built once, at construction — on the theme it was
// created under, so after a Theme-editor surfaces pick it alone kept the old
// body color.
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

// rowToggleLabelColor is the letter/glyph color for a row toggle: muted at
// rest, and on an active fill whichever of on-surface / background reads
// better against it.
func rowToggleLabelColor(base color.Color, role rowToggleRole, on bool) color.Color {
	if !on || role == roleKebab {
		return colTextSecondary
	}
	return legibleOn(rowToggleFillRGBA(base, role, on))
}

// applyRowToggle puts a row toggle into its on/off visual: body, letter
// color, and the pressed STATE flag the controls cache compares against.
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

// rowKebabStyle returns the per-row ellipsis (kebab) chip style: the neutral
// row-control body. The overflow affordance has no state, so it carries no
// instrument color (the row's identity is on its label swatch).
func rowKebabStyle(base color.Color) rowToggleVisual {
	return rowToggleStyle(base, roleKebab, false)
}

// rowShadeToRGBA narrows a color.Color (as returned by adjustColor /
// WithAlphaFromColor) to a concrete premultiplied RGBA.
func rowShadeToRGBA(c color.Color) color.RGBA {
	return color.RGBAModel.Convert(c).(color.RGBA)
}

// legibleOn returns on-surface or background — the two sanctioned label
// colors — whichever has the higher WCAG contrast ratio against fill.
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
