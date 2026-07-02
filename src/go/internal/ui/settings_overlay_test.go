package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// findHitAreaByTag returns the first hit area whose Tag matches, ok=false if none.
func findHitAreaByTag(areas []HitArea, tag string) (HitArea, bool) {
	for _, a := range areas {
		if a.Tag == tag {
			return a, true
		}
	}
	return HitArea{}, false
}

// TestSettingsOverlayPanelRoutesToPills checks per-pill routing: pressing a
// pill's own hit area picks that language via its Button.OnClick; a press on
// panel background is consumed by the catch-all without picking. (The full
// real-dispatch path is covered by TestSettingsLanguageButtonRealClickDispatch —
// this is the unit-level router check.)
func TestSettingsOverlayPanelRoutesToPills(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	var picked i18n.Locale
	pickedAny := false
	o := NewSettingsOverlay(func(l i18n.Locale) { picked = l; pickedAny = true })
	screen := image.Rect(0, 0, 1200, 800)
	o.Layout(screen, screen)

	enArea, ok := findHitAreaByTag(o.HitAreas(), "settings-lang-en")
	if !ok {
		t.Fatal("no settings-lang-en hit area")
	}
	esArea, ok := findHitAreaByTag(o.HitAreas(), "settings-lang-es")
	if !ok {
		t.Fatal("no settings-lang-es hit area")
	}

	en, es := o.LanguagePillRects()
	if en.Empty() || es.Empty() {
		t.Fatalf("pill rects empty: en=%v es=%v", en, es)
	}

	center := func(r image.Rectangle) (int, int) { return (r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2 }

	x, y := center(es)
	esArea.Handler.OnPress(x, y)
	esArea.Handler.OnRelease(x, y)
	if !pickedAny || picked != i18n.LocaleES {
		t.Fatalf("press in es pill: picked=%q any=%v want es", picked, pickedAny)
	}

	x, y = center(en)
	enArea.Handler.OnPress(x, y)
	enArea.Handler.OnRelease(x, y)
	if picked != i18n.LocaleEN {
		t.Fatalf("press in en pill: picked=%q want en", picked)
	}

	// Panel background (top-left corner, well above the pills) must not pick.
	bgArea, ok := findHitAreaByTag(o.HitAreas(), "settings-panel")
	if !ok {
		t.Fatal("no settings-panel catch-all hit area")
	}
	pickedAny = false
	bgArea.Handler.OnPress(o.rect.Min.X+2, o.rect.Min.Y+2)
	if pickedAny {
		t.Fatalf("panel-background press should not pick a language, got %q", picked)
	}
}

func TestSettingsOverlayDrawDoesNotPanic(t *testing.T) {
	o := NewSettingsOverlay(func(i18n.Locale) {})
	screen := image.Rect(0, 0, 1200, 800)
	o.Layout(screen, screen)
	// Draw must run cleanly under -tags test (stub backend).
	o.Draw(nil) // Draw guards a nil screen and returns early.
}
