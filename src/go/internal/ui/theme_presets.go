package ui

import "github.com/ingyamilmolinar/beatmo/internal/i18n"

// ThemePreset is a first-party, curated theme: one swatch per category
// (theme_categories.go). Presets are the primary way users pick a look; the
// per-category chips under "Adjust colors" customize on top of one.
type ThemePreset struct {
	ID         string
	NameKey    i18n.Key
	GlossKey   i18n.Key
	Categories map[string]string
}

// themePresets is the single table (order = editor order; first = default).
// Every preset keeps the dark, readable structure: the KeepLightness
// categories only take a hue/tint from their swatch.
var themePresets = []ThemePreset{
	{ID: "vice-city", NameKey: i18n.KeyThemePresetViceCity, GlossKey: i18n.KeyThemePresetViceCityGloss, Categories: map[string]string{
		// The shipped DESIGN.md palette, with one refinement: the focus ring is
		// hot-pink-400 (#EE00DD), the Vice City signature magenta the palette
		// spec names, instead of the lighter -300.
		"background": "orchid-500", "surfaces": "orchid-500", "text": "orchid-100", "accent": "sunset-gold-300",
		"focus": "hot-pink-400", "grid": "orchid-500", "nodes": "orchid-500", "dividers": "orchid-100", "viz": "sunset-gold-300"}},
	{ID: "ocean", NameKey: i18n.KeyThemePresetOcean, GlossKey: i18n.KeyThemePresetOceanGloss, Categories: map[string]string{
		"background": "electric-blue-500", "surfaces": "electric-blue-400", "text": "cyan-100", "accent": "tangerine-300",
		"focus": "hot-pink-300", "grid": "electric-blue-300", "nodes": "cyan-400", "dividers": "cyan-200", "viz": "cyan-200"}},
	{ID: "midnight", NameKey: i18n.KeyThemePresetMidnight, GlossKey: i18n.KeyThemePresetMidnightGloss, Categories: map[string]string{
		"background": "electric-blue-500", "surfaces": "electric-blue-500", "text": "cyan-100", "accent": "cyan-300",
		"focus": "hot-pink-300", "grid": "electric-blue-400", "nodes": "cyan-300", "dividers": "cyan-300", "viz": "cyan-200"}},
	{ID: "ember", NameKey: i18n.KeyThemePresetEmber, GlossKey: i18n.KeyThemePresetEmberGloss, Categories: map[string]string{
		"background": "tangerine-500", "surfaces": "tangerine-500", "text": "sunset-gold-100", "accent": "tangerine-300",
		"focus": "hot-pink-300", "grid": "tangerine-400", "nodes": "sunset-gold-400", "dividers": "tangerine-200", "viz": "sunset-gold-300"}},
	{ID: "neon-mint", NameKey: i18n.KeyThemePresetNeonMint, GlossKey: i18n.KeyThemePresetNeonMintGloss, Categories: map[string]string{
		"background": "mint-500", "surfaces": "mint-500", "text": "mint-100", "accent": "mint-300",
		"focus": "hot-pink-300", "grid": "mint-400", "nodes": "cyan-300", "dividers": "mint-200", "viz": "cyan-200"}},
	{ID: "orchid", NameKey: i18n.KeyThemePresetOrchid, GlossKey: i18n.KeyThemePresetOrchidGloss, Categories: map[string]string{
		"background": "orchid-500", "surfaces": "orchid-400", "text": "orchid-100", "accent": "orchid-300",
		"focus": "hot-pink-300", "grid": "orchid-400", "nodes": "cyan-300", "dividers": "orchid-200", "viz": "cyan-200"}},
}

// defaultThemePresetID is the preset an unset document reads as.
const defaultThemePresetID = "vice-city"

func themePresetByID(id string) (ThemePreset, bool) {
	for _, p := range themePresets {
		if p.ID == id {
			return p, true
		}
	}
	return ThemePreset{}, false
}
