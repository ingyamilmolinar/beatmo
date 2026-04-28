package ui

import "image/color"

// theme_tokens.go — Semantic token accessors for the Tunkul UI palette.
//
// Rule: use these instead of raw colXxx variables from theme.go so the
// palette can be updated in one place. One token per semantic role — never
// add a token that duplicates an existing role; update the existing one.

// ── Surfaces ──────────────────────────────────────────────────────────────

func TokenSurface1() color.RGBA { return colSurface1 }
func TokenSurface2() color.RGBA { return colSurface2 }
func TokenSurface3() color.RGBA { return colSurface3 }
// TokenPanelBG carries an intentional non-255 alpha (panel-near-opaque,
// 250) so callers can composite the panel surface against the scrim.
func TokenPanelBG() color.NRGBA { return colPanelBG }

// ── Accent ────────────────────────────────────────────────────────────────

func TokenAccent() color.RGBA       { return colAccent }
func TokenAccentBright() color.RGBA { return colAccentBright }
func TokenAccentDim() color.RGBA    { return colAccentDim }

// ── Text ──────────────────────────────────────────────────────────────────

func TokenTextPrimary() color.RGBA   { return colTextPrimary }
func TokenTextSecondary() color.RGBA { return colTextSecondary }
func TokenTextDisabled() color.RGBA  { return colTextDisabled }
func TokenTextAccent() color.RGBA    { return colTextAccent }

// ── Semantic state ────────────────────────────────────────────────────────

// TokenPlayGreen is the icon tint for the play button in active/playing state.
func TokenPlayGreen() color.RGBA { return colPlayGreen }

// TokenStopRed is the icon tint for the stop button and error text.
// Never use as a button fill — use TokenDeleteFill for destructive fills.
func TokenStopRed() color.RGBA { return colStopRed }

// TokenMuteActiveFill is the fill color for an active mute button.
// Distinct from TokenStopRed (stop text) and TokenDeleteFill (destructive fill).
func TokenMuteActiveFill() color.RGBA { return colMuteActive }

// TokenDeleteFill is the fill for destructive action buttons (delete confirm, etc.).
func TokenDeleteFill() color.RGBA { return colDeleteFill }

// TokenSoloActiveFill is the fill for an active solo button.
func TokenSoloActiveFill() color.RGBA { return colSoloActive }

// ── Borders ───────────────────────────────────────────────────────────────

func TokenBorderSubtle() color.NRGBA {
	r, g, b, a := colBorderSubtle.RGBA()
	return color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func TokenBorderMedium() color.NRGBA {
	r, g, b, a := colBorderMedium.RGBA()
	return color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

// ── Alpha ─────────────────────────────────────────────────────────────────
//
// Stitch's design.md spec only allows opaque #RRGGBB tokens, so semi-
// transparent compositions cannot be encoded as colors. These named alpha
// constants pair with WithAlpha() so transparent fills use a small,
// auditable set of opacity buckets instead of inline magic numbers.

const (
	AlphaFaint        = genAlphaFaint        // ghost overlays, hover hints, fuzzy highlight
	AlphaSubtle       = genAlphaSubtle       // dimmed indicators, hover scrims
	AlphaMedium       = genAlphaMedium       // panel scrims, soft separators
	AlphaStrong       = genAlphaStrong       // focus rings, prominent overlays
	AlphaOverlay      = genAlphaOverlay      // panel backgrounds, modal scrims
	AlphaPanelBorder  = genAlphaBorderPanel  // floating-panel chrome border (20)
	AlphaRowActive    = genAlphaRowActive    // row-rack active highlight (~60%, 153)
	AlphaRowRackZebra = genAlphaRowRackZebra // row-rack zebra-stripe overlay + drum-cell border (10)
)

// WithAlpha returns the given RGBA color with its alpha channel replaced by a.
// Use the named Alpha* constants instead of raw integers so opacity choices
// stay legible and auditable.
func WithAlpha(c color.RGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

// WithAlphaNRGBA returns the given NRGBA color with its alpha channel
// replaced by a. Sibling of WithAlpha for inputs that are already NRGBA
// (e.g., colors returned from WithAlpha that need re-alpha-binding).
func WithAlphaNRGBA(c color.NRGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

// WithAlphaFromColor returns an NRGBA composed of the RGB channels of c
// (resolved via the standard color.Color interface) and the given alpha.
// Use when c is a generic color.Color whose concrete type is unknown — for
// example, a per-row instrument color sourced from import or theme palette.
func WithAlphaFromColor(c color.Color, a uint8) color.NRGBA {
	if c == nil {
		return color.NRGBA{}
	}
	r, g, b, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: a}
}
