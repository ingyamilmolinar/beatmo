package ui

import (
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// ThemeCategory groups DESIGN.md color tokens that recolor together from ONE
// picked swatch. This table is the single source of truth for the Theme
// overlay rows, the derivation engine (theme_derive.go), and the guard
// TestThemeCategoryTableComplete (every color token is in exactly one
// category or in themeFixedTokens).
type ThemeCategory struct {
	ID       string
	LabelKey i18n.Key
	GlossKey i18n.Key
	// Anchor is the swatch NAME whose HSL the default token values are
	// measured against. Applying the anchor reproduces DESIGN.md exactly.
	Anchor string
	// KeepLightness categories keep each token's absolute lightness (the
	// shade column only chooses tint strength) so a light pick never yields a
	// light UI or unreadable text.
	KeepLightness bool
	// Tokens: the FIRST entry is the category's principal color, shown on the
	// editor chip (background → the canvas, accent → primary, …).
	Tokens []string
}

var themeCategories = []ThemeCategory{
	{ID: "background", LabelKey: i18n.KeyThemeCatBackground, GlossKey: i18n.KeyThemeGlossBackground, Anchor: "orchid-500", KeepLightness: true, Tokens: []string{
		"background", "grid-horizon", "transport-bar-bg", "rack-bed", "toolbar-bed", "timeline-total-bg", "viz-bg", "viz-scope-bg", "viz-meter-bg",
		"drum-stripe-even", "drum-stripe-odd", "drum-cell-off", "eq-readout-bg", "sidebar-badge-bg", "viz-pill-fill"}},
	{ID: "surfaces", LabelKey: i18n.KeyThemeCatSurfaces, GlossKey: i18n.KeyThemeGlossSurfaces, Anchor: "orchid-500", KeepLightness: true, Tokens: []string{
		"surface-1", "surface-2", "surface-3", "surface-overlay", "disabled-fill", "fx-active-fill",
		"sidebar-section-bg", "sidebar-chip-fill", "slider-track-fill", "scope-label-dim"}},
	{ID: "text", LabelKey: i18n.KeyThemeCatText, GlossKey: i18n.KeyThemeGlossText, Anchor: "orchid-100", KeepLightness: true, Tokens: []string{
		"on-surface", "on-surface-muted", "on-surface-disabled", "popup-text-secondary", "slider-thumb-fill",
		"sidebar-swatch-fallback", "row-rack-color-fallback", "drum-cell-highlight", "drum-mute-highlight", "divider-base"}},
	{ID: "accent", LabelKey: i18n.KeyThemeCatAccent, GlossKey: i18n.KeyThemeGlossAccent, Anchor: "sunset-gold-300", Tokens: []string{
		"primary", "primary-bright", "primary-dim", "on-surface-accent", "splitter-handle", "drum-glow", "slider-fill",
		"solo-active-border", "eq-filter-active-fill", "eq-filter-active-border", "viz-glow",
		// The timeline ruler's visible window and the playhead are transport
		// chrome beside the accent-lit controls, not a data canvas: under a
		// preset whose Visualizers hue differs from its Accent (Ocean) they
		// read as a stray third color when grouped with the traces.
		"timeline-view", "timeline-view-hi", "timeline-cursor"}},
	{ID: "focus", LabelKey: i18n.KeyThemeCatFocus, GlossKey: i18n.KeyThemeGlossFocus, Anchor: "hot-pink-300", Tokens: []string{"focus-ring"}},
	{ID: "grid", LabelKey: i18n.KeyThemeCatGrid, GlossKey: i18n.KeyThemeGlossGrid, Anchor: "orchid-500", KeepLightness: true, Tokens: []string{
		"grid-line", "grid-half", "grid-quarter", "grid-eighth", "grid-sixteenth", "grid-thirty-second"}},
	{ID: "nodes", LabelKey: i18n.KeyThemeCatNodes, GlossKey: i18n.KeyThemeGlossNodes, Anchor: "orchid-500", KeepLightness: true, Tokens: []string{
		"node-fill", "node-border", "edge-color", "drum-mute-cell", "timeline-beat"}},
	{ID: "dividers", LabelKey: i18n.KeyThemeCatDividers, GlossKey: i18n.KeyThemeGlossDividers, Anchor: "orchid-100", KeepLightness: true, Tokens: []string{
		"divider-highlight", "border", "divider-shadow", "sidebar-chip-border", "viz-pill-border", "eq-zero-line", "wave-trace-dry"}},
	{ID: "viz", LabelKey: i18n.KeyThemeCatViz, GlossKey: i18n.KeyThemeGlossViz, Anchor: "sunset-gold-300", Tokens: []string{
		"viz-bar", "viz-bar-peak", "viz-curve", "viz-wave-a", "viz-wave-b", "viz-scope-trace-a", "viz-scope-trace-b",
		"viz-scope-trace-diff", "viz-spectrum-peak-marker", "viz-meter-low", "viz-meter-mid",
		"viz-bass", "viz-mids", "viz-treble"}},
}

// themeFixedTokens are semantic-state and debug colors that a theme never
// touches (red must stay "danger", green must stay "go").
var themeFixedTokens = map[string]bool{
	"success": true, "error": true, "error-text": true, "mute": true, "mute-active-border": true,
	"destructive": true, "destructive-border": true, "destructive-confirm-fill": true, "destructive-confirm-border": true,
	"record-idle": true, "record-active": true, "play-desktop-fill": true, "play-desktop-border": true,
	"stop-desktop-fill": true, "stop-desktop-border": true, "eq-mute-active-fill": true, "menu-delete-tint": true,
	"viz-meter-high": true, "viz-meter-clip": true, "viz-debug-edge": true, "viz-debug-node": true, "dim-black": true,
	"slider-thumb-shadow": true, // pure-black drop shadow, achromatic by design
}

// isRawPaletteToken reports whether name is one of the raw chromatic ramp /
// night-neutral entries (the palette itself, not a role). They are the
// SOURCE swatches and are never themed.
func isRawPaletteToken(name string) bool {
	if strings.HasPrefix(name, "night-") {
		return true
	}
	for _, fam := range []string{"sunset-gold-", "tangerine-", "hot-pink-", "orchid-", "electric-blue-", "cyan-", "mint-"} {
		if strings.HasPrefix(name, fam) {
			return true
		}
	}
	return false
}

// isSwatchName reports whether name is one of the curated picker swatches.
func isSwatchName(name string) bool {
	for _, s := range SuggestedInstrumentSwatches {
		if s.Name == name {
			return true
		}
	}
	return false
}

func themeCategoryByID(id string) (ThemeCategory, bool) {
	for _, c := range themeCategories {
		if c.ID == id {
			return c, true
		}
	}
	return ThemeCategory{}, false
}

// themeCategoryOfToken returns the category id owning token ("", false for
// fixed / raw / unknown tokens).
func themeCategoryOfToken(token string) (string, bool) {
	for _, c := range themeCategories {
		for _, t := range c.Tokens {
			if t == token {
				return c.ID, true
			}
		}
	}
	return "", false
}
