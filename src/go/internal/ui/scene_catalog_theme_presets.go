//go:build !noscenes

package ui

import "strings"

// scene_catalog_theme_presets.go — one screenshot scene per first-party theme
// preset, driven through the SAME path a user takes: open the Theme editor and
// press that preset's row button. Kept as a regression surface for the visual
// audit of every preset (buttons, sections, data canvases must all re-theme).
func init() {
	for _, p := range themePresets {
		id := p.ID
		sceneCatalog = append(sceneCatalog, Scene{
			// Scene names are snake_case (TestSceneCatalogNamesUniqueAndKebabCase).
			Name:         "theme_pick_" + strings.ReplaceAll(id, "-", "_"),
			Description:  "theme editor open with the " + id + " preset row pressed (theme applied to the whole UI)",
			SettleFrames: 30,
			Setup: func(g *Game) {
				g.toggleThemeOverlay()
				if ov, ok := g.drum.portal().TopOverlay().(*ThemeOverlay); ok {
					if b := ov.presetBtns[id]; b != nil && b.OnClick != nil {
						b.OnClick()
					}
				}
			},
		})
	}
}
