//go:build !noscenes

package ui

import "strings"

// One scene per preset through the user's own path (open the editor, press the row), so every surface must
// re-theme.
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
