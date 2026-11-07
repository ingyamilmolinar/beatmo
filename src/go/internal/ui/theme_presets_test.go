//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

func TestThemePresetsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range themePresets {
		if p.ID == "" || seen[p.ID] {
			t.Fatalf("preset id %q empty or duplicated", p.ID)
		}
		seen[p.ID] = true
		if p.NameKey == "" || p.GlossKey == "" {
			t.Fatalf("preset %s lacks i18n keys", p.ID)
		}
		for _, c := range themeCategories {
			name, ok := p.Categories[c.ID]
			if !ok {
				t.Fatalf("preset %s misses category %s", p.ID, c.ID)
			}
			if !isSwatchName(name) {
				t.Fatalf("preset %s category %s: %q is not a swatch", p.ID, c.ID, name)
			}
		}
		if len(p.Categories) != len(themeCategories) {
			t.Fatalf("preset %s names %d categories, want %d", p.ID, len(p.Categories), len(themeCategories))
		}
	}
	if themePresets[0].ID != defaultThemePresetID {
		t.Fatalf("first preset must be the default %q", defaultThemePresetID)
	}
}

// Vice City is the shipped palette plus one refinement: the hot-pink-400 focus ring.
func TestVicePresetIsShippedPaletteButFocusRing(t *testing.T) {
	got, warns := resolveThemeTokens(theme.Doc{Preset: "vice-city"})
	if len(warns) != 0 {
		t.Fatalf("warnings: %v", warns)
	}
	pink400, _ := swatchRGBA("hot-pink-400")
	for name, def := range genColorDefaultByName {
		if name == "focus-ring" {
			if got[name] != pink400 {
				t.Fatalf("focus-ring = %v, want hot-pink-400 %v", got[name], pink400)
			}
			continue
		}
		if got[name] != def {
			t.Errorf("vice-city changed %s: %v → %v", name, def, got[name])
		}
	}
}

func TestPresetResolutionOrder(t *testing.T) {
	got, warns := resolveThemeTokens(theme.Doc{Preset: "ocean", Categories: map[string]string{"accent": "orchid-300"}, Tokens: map[string]string{"focus-ring": "#112233"}})
	if len(warns) != 0 {
		t.Fatalf("warnings: %v", warns)
	}
	ocean, _ := themePresetByID("ocean")
	bgPick, _ := swatchRGBA(ocean.Categories["background"])
	bh, _, _ := rgbToHSL(bgPick)
	gh, _, _ := rgbToHSL(got["background"])
	if hueDistance(bh, gh) > 20 {
		t.Fatalf("preset background not applied: %v", got["background"])
	}
	orchid, _ := swatchRGBA("orchid-300")
	if got["primary"] != orchid {
		t.Fatalf("document category must override the preset: primary=%v", got["primary"])
	}
	if got["focus-ring"].R != 0x11 {
		t.Fatalf("token must override everything: %v", got["focus-ring"])
	}
	if _, w := resolveThemeTokens(theme.Doc{Preset: "no-such"}); len(w) != 1 {
		t.Fatalf("unknown preset must warn once: %v", w)
	}
}

func TestPresetPrecedenceFollowsCategoryRule(t *testing.T) {
	u := theme.UserDoc{Doc: theme.Doc{Preset: "ocean"}}
	c := theme.Doc{Preset: "ember"}
	if effectivePresetID(u, c) != "ember" {
		t.Fatal("no override: circuit preset wins")
	}
	u.OverrideCircuits = true
	if effectivePresetID(u, c) != "ocean" {
		t.Fatal("override: user preset wins")
	}
	if effectivePresetID(theme.UserDoc{}, theme.Doc{}) != "" {
		t.Fatal("nothing set → empty")
	}
	if effectivePresetID(u, theme.Doc{}) != "ocean" {
		t.Fatal("circuit unset → user")
	}
}

func TestPresetPickClearsCustomizations(t *testing.T) {
	defer SetThemeSink(nil)
	sink := &fakeThemeSink{}
	SetThemeSink(sink)
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	g.setUserThemeCategory("accent", "orchid-300")
	g.setUserThemePreset("ocean")
	u := g.UserTheme()
	if u.Preset != "ocean" || len(u.Categories) != 0 || len(u.Tokens) != 0 {
		t.Fatalf("preset pick must clear customizations: %+v", u)
	}
	tang, _ := swatchRGBA("tangerine-300")
	if genColorPrimary != tang {
		t.Fatalf("ocean accent not applied: %v", genColorPrimary)
	}
	if len(sink.saved) == 0 || sink.saved[len(sink.saved)-1].Preset != "ocean" {
		t.Fatalf("preset not persisted: %+v", sink.saved)
	}
	g.setCircuitThemePreset("ember")
	if g.undoManager.UndoLabel() == "" {
		t.Fatal("circuit preset pick must be undoable")
	}
	if genColorPrimary != tang { // user has no override → circuit wins → tangerine-300 is ember's accent too
		t.Fatalf("circuit preset not applied: %v", genColorPrimary)
	}
}
