package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestSettingsOverlayPanelRoutesToPills checks the single-panel hit handler's
// internal routing: a press inside a pill rect picks that language; a press on
// panel background is consumed without picking. (The full real-dispatch path is
// covered by TestSettingsLanguagePillRealClickDispatch — this is the unit-level
// router check.)
func TestSettingsOverlayPanelRoutesToPills(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	var picked i18n.Locale
	pickedAny := false
	o := NewSettingsOverlay(func(l i18n.Locale) { picked = l; pickedAny = true })
	screen := image.Rect(0, 0, 1200, 800)
	o.Layout(screen, screen)

	hits := o.HitAreas()
	if len(hits) != 1 {
		t.Fatalf("expected exactly 1 full-panel hit area (portal flattens z), got %d", len(hits))
	}
	h := hits[0].Handler
	if h == nil {
		t.Fatal("panel hit area has no handler")
	}

	en, es := o.LanguagePillRects()
	if en.Empty() || es.Empty() {
		t.Fatalf("pill rects empty: en=%v es=%v", en, es)
	}

	center := func(r image.Rectangle) (int, int) { return (r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2 }

	x, y := center(es)
	h.OnPress(x, y)
	if !pickedAny || picked != i18n.LocaleES {
		t.Fatalf("press in es pill: picked=%q any=%v want es", picked, pickedAny)
	}

	x, y = center(en)
	h.OnPress(x, y)
	if picked != i18n.LocaleEN {
		t.Fatalf("press in en pill: picked=%q want en", picked)
	}

	// Panel background (top-left corner, well above the pills) must not pick.
	pickedAny = false
	h.OnPress(o.rect.Min.X+2, o.rect.Min.Y+2)
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
