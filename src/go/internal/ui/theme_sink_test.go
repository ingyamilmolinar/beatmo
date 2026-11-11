//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

type fakeThemeSink struct {
	stored theme.UserDoc
	saved  []theme.UserDoc
}

func (f *fakeThemeSink) LoadTheme() (theme.UserDoc, error) { return f.stored, nil }
func (f *fakeThemeSink) SaveTheme(d theme.UserDoc) error {
	f.saved = append(f.saved, d.Clone())
	return nil
}

func TestApplyStoredThemeBeforeFirstFrame(t *testing.T) {
	defer SetThemeSink(nil)
	sink := &fakeThemeSink{stored: theme.UserDoc{Doc: theme.Doc{Categories: map[string]string{"accent": "mint-300"}}}}
	SetThemeSink(sink)
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	ApplyStoredTheme(g)
	mint, _ := swatchRGBA("mint-300")
	if genColorPrimary != mint {
		t.Fatalf("stored theme not applied: primary=%v", genColorPrimary)
	}
	if got := g.UserTheme().Categories["accent"]; got != "mint-300" {
		t.Fatalf("Game.userTheme not seeded: %q", got)
	}
}

func TestUserPickPersistsThroughSink(t *testing.T) {
	defer SetThemeSink(nil)
	sink := &fakeThemeSink{}
	SetThemeSink(sink)
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	g.setUserThemeCategory("focus", "cyan-300")
	g.setThemeOverride(true)
	if len(sink.saved) != 2 || sink.saved[0].Categories["focus"] != "cyan-300" || !sink.saved[1].OverrideCircuits {
		t.Fatalf("saved = %+v", sink.saved)
	}
}
