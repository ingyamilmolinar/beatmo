//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

func TestThemePrecedenceTable(t *testing.T) {
	cases := []struct {
		name          string
		override      bool
		user, circuit string // "" = unset
		want          string
	}{
		{"nothing set", false, "", "", ""},
		{"user only", false, "mint-100", "", "mint-100"},
		{"circuit only", false, "", "cyan-200", "cyan-200"},
		{"both, no override → circuit", false, "mint-100", "cyan-200", "cyan-200"},
		{"override, nothing", true, "", "", ""},
		{"override, user only", true, "mint-100", "", "mint-100"},
		{"override, circuit only → circuit", true, "", "cyan-200", "cyan-200"},
		{"override, both → user", true, "mint-100", "cyan-200", "mint-100"},
	}
	for _, c := range cases {
		u := theme.UserDoc{OverrideCircuits: c.override}
		if c.user != "" {
			u.Categories = map[string]string{"accent": c.user}
		}
		var cd theme.Doc
		if c.circuit != "" {
			cd.Categories = map[string]string{"accent": c.circuit}
		}
		got := effectiveTheme(u, cd).Categories["accent"]
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestEffectiveThemeMergesTokensWithSameRule(t *testing.T) {
	u := theme.UserDoc{Doc: theme.Doc{Tokens: map[string]string{"primary": "#111111"}}}
	c := theme.Doc{Tokens: map[string]string{"primary": "#222222", "focus-ring": "#333333"}}
	got := effectiveTheme(u, c)
	if got.Tokens["primary"] != "#222222" || got.Tokens["focus-ring"] != "#333333" {
		t.Fatalf("no override: %v", got.Tokens)
	}
	u.OverrideCircuits = true
	if got := effectiveTheme(u, c); got.Tokens["primary"] != "#111111" {
		t.Fatalf("override: %v", got.Tokens)
	}
}

func TestEditorPickClearsCategoryTokens(t *testing.T) {
	d := theme.Doc{Tokens: map[string]string{"primary": "#111111", "focus-ring": "#222222"}}
	dropTokensOfCategory(&d, "accent")
	if _, ok := d.Tokens["primary"]; ok {
		t.Fatal("primary belongs to accent and must be dropped")
	}
	if d.Tokens["focus-ring"] != "#222222" {
		t.Fatal("focus-ring belongs to focus and must survive")
	}
}

func TestRecomputeSkipsApplyWhenUnchanged(t *testing.T) {
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	gen := ThemeGeneration()
	g.recomputeEffectiveTheme() // nothing set → identical to active → no apply
	if ThemeGeneration() != gen {
		t.Fatal("recompute applied an unchanged theme")
	}
	g.setUserThemeCategory("accent", "orchid-300")
	if ThemeGeneration() != gen+1 || genColorPrimary == genColorDefaultByName["primary"] {
		t.Fatal("user pick did not apply")
	}
	g.setThemeOverride(true) // still the same effective doc → no new generation
	if ThemeGeneration() != gen+1 {
		t.Fatal("override flip with no circuit theme must not re-apply")
	}
	g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"accent": "mint-300"}})
	if ThemeGeneration() != gen+1 { // override on → user still wins → unchanged
		t.Fatal("override on: circuit theme must not change the effective theme")
	}
	g.setThemeOverride(false)
	mint, _ := swatchRGBA("mint-300")
	if genColorPrimary != mint {
		t.Fatalf("override off: circuit should win, primary=%v", genColorPrimary)
	}
	g.resetUserTheme()
	g.resetCircuitTheme()
	if genColorPrimary != genColorDefaultByName["primary"] {
		t.Fatal("resets did not return to default")
	}
}
