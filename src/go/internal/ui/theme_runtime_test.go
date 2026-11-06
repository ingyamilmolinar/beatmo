//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

func TestApplyThemeRewritesTokensSpecsAndAliases(t *testing.T) {
	restore := SetThemeForTest(theme.Doc{Categories: map[string]string{"accent": "orchid-300"}})
	defer restore()
	pick, _ := swatchRGBA("orchid-300")
	if genColorPrimary != pick {
		t.Fatalf("genColorPrimary = %v, want %v", genColorPrimary, pick)
	}
	if colAccent != pick || SignalUI.Color != pick {
		t.Fatalf("aliases not rebuilt: colAccent=%v SignalUI=%v", colAccent, SignalUI.Color)
	}
	if Spec(ComponentButtonPrimary).Fill != pick {
		t.Fatalf("component specs not rebuilt: %v", Spec(ComponentButtonPrimary).Fill)
	}
	if FABStyle.Fill != pick {
		t.Fatalf("spec-derived style not rebuilt: %v", FABStyle.Fill)
	}
	if PrimaryActionStyle.Fill != pick {
		t.Fatalf("folded style not rebuilt: %v", PrimaryActionStyle.Fill)
	}
	restore()
	if genColorPrimary != genColorDefaultByName["primary"] || Spec(ComponentButtonPrimary).Fill != genColorDefaultByName["primary"] {
		t.Fatal("restore did not return to defaults")
	}
}

func TestApplyThemeNotifiesListenersAndBumpsGeneration(t *testing.T) {
	calls := 0
	cancel := OnThemeChange(func() { calls++ })
	defer cancel()
	gen := ThemeGeneration()
	restore := SetThemeForTest(theme.Doc{Categories: map[string]string{"focus": "mint-300"}})
	defer restore()
	if calls != 1 || ThemeGeneration() != gen+1 {
		t.Fatalf("calls=%d gen %d→%d", calls, gen, ThemeGeneration())
	}
	if !ActiveTheme().Equal(theme.Doc{Categories: map[string]string{"focus": "mint-300"}}) {
		t.Fatalf("ActiveTheme = %+v", ActiveTheme())
	}
	restore()
	if calls != 2 {
		t.Fatalf("restore must notify listeners too: calls=%d", calls)
	}
	cancel()
	SetThemeForTest(theme.Doc{})()
	if calls != 2 {
		t.Fatalf("cancelled listener still called: calls=%d", calls)
	}
}

// A spec button whose icon color came from its spec must follow the theme;
// one whose call site overrode IconColor must keep the override.
func TestSpecButtonIconColorFollowsTheme(t *testing.T) {
	b := NewSpecButton("", ComponentButtonSecondary, nil) // iconColor: on-surface-muted
	b.Icon = string(IconSettings)
	override := NewSpecButton("", ComponentButtonSecondary, nil)
	override.Icon = string(IconSettings)
	override.IconColor = color.RGBA{1, 2, 3, 255}
	restore := SetThemeForTest(theme.Doc{Categories: map[string]string{"text": "mint-300"}})
	defer restore()
	want := Spec(ComponentButtonSecondary).IconColor
	if want == genColorDefaultByName["on-surface-muted"] {
		t.Fatal("test precondition: theme did not change on-surface-muted")
	}
	if got := b.effectiveIconColor(); got != want {
		t.Fatalf("spec-seeded icon color = %v, want themed %v", got, want)
	}
	if got := override.effectiveIconColor(); got != (color.RGBA{1, 2, 3, 255}) {
		t.Fatalf("call-site override lost: %v", got)
	}
}
