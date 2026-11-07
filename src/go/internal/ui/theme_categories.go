package ui

import (
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The single source for the editor rows and the derivation; every colour token is in exactly one category or in
// themeFixedTokens.
type ThemeCategory struct {
	ID       string
	LabelKey i18n.Key
	GlossKey i18n.Key
	// Applying the anchor reproduces DESIGN.md exactly.
	Anchor string
	// Absolute lightness is kept (the shade only sets tint strength), so a light pick never makes a light UI or
	// unreadable text.
	KeepLightness bool
	// The FIRST entry is the principal colour shown on the editor chip.
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
		// Transport chrome, not a data canvas: grouped with the traces they read as a stray third colour under a preset
		// whose Visualizers hue differs from Accent.
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

// Red must stay "danger" and green "go", whatever the theme.
var themeFixedTokens = map[string]bool{
	"success": true, "error": true, "error-text": true, "mute": true, "mute-active-border": true,
	"destructive": true, "destructive-border": true, "destructive-confirm-fill": true, "destructive-confirm-border": true,
	"record-idle": true, "record-active": true, "play-desktop-fill": true, "play-desktop-border": true,
	"stop-desktop-fill": true, "stop-desktop-border": true, "eq-mute-active-fill": true, "menu-delete-tint": true,
	"viz-meter-high": true, "viz-meter-clip": true, "viz-debug-edge": true, "viz-debug-node": true, "dim-black": true,
	"slider-thumb-shadow": true, // pure-black drop shadow, achromatic by design
}

// The SOURCE swatches, never themed themselves.
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

// !ok for fixed, raw or unknown tokens.
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
