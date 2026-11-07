package ui

import "image/color"

// Use these, not raw colXxx vars, so the palette changes in one place; never add a token for a role that has one.

// ── Surfaces ──────────────────────────────────────────────────────────────

func TokenSurface1() color.RGBA { return colSurface1 }
func TokenSurface2() color.RGBA { return colSurface2 }
func TokenSurface3() color.RGBA { return colSurface3 }

// Intentionally translucent so callers can composite against the scrim.
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

// Icon tint and error text; never a fill (destructive fills use TokenDeleteFill).
func TokenStopRed() color.RGBA { return colStopRed }

// Distinct from stop red (text) and delete fill (destructive).
func TokenMuteActiveFill() color.RGBA { return colMuteActive }

// TokenDeleteFill is the fill for destructive action buttons (delete confirm, etc.).
func TokenDeleteFill() color.RGBA { return colDeleteFill }

// TokenSoloActiveFill is the fill for an active solo button.
func TokenSoloActiveFill() color.RGBA { return colSoloActive }

// Pair with WithAlpha + AlphaSubtle behind data; opaque draws would compete with the bars.

func TokenVizBass() color.RGBA   { return colVizBass }
func TokenVizMids() color.RGBA   { return colVizMids }
func TokenVizTreble() color.RGBA { return colVizTreble }

// ── Borders ───────────────────────────────────────────────────────────────

func TokenBorderSubtle() color.NRGBA {
	r, g, b, a := colBorderSubtle.RGBA()
	return color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func TokenBorderMedium() color.NRGBA {
	r, g, b, a := colBorderMedium.RGBA()
	return color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

// design.md tokens are opaque #RRGGBB only, so translucency uses this small auditable set of buckets.

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

// Use the named Alpha* buckets, never raw numbers.
func WithAlpha(c color.RGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

func WithAlphaNRGBA(c color.NRGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

// For a color.Color of unknown concrete type, e.g. an imported row colour.
func WithAlphaFromColor(c color.Color, a uint8) color.NRGBA {
	if c == nil {
		return color.NRGBA{}
	}
	r, g, b, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: a}
}
