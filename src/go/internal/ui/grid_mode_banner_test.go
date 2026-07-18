package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The armed-mode helper banners are localized, parameterized, and
// Esc-hint-gated through one pure helper shared with the draw path.
func TestGridModeBannerText(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	n := &uiNode{I: 2, J: 4}

	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		i18n.SetLocale(loc)
		for _, kind := range []GridModeKind{GridModeMove, GridModeCopy, GridModeConnect} {
			got := gridModeBannerText(kind, n, false)
			if got == "" {
				t.Fatalf("[%v] %v banner empty", loc, kind)
			}
			if strings.Contains(got, "%d") || strings.Contains(got, "%!") {
				t.Fatalf("[%v] %v banner has unformatted verb: %q", loc, kind, got)
			}
			// Move and connect embed the subject's grid coordinate.
			if kind != GridModeCopy && !strings.Contains(got, "(2,4)") {
				t.Fatalf("[%v] %v banner missing subject coords: %q", loc, kind, got)
			}
			// Esc hint appends the localized suffix exactly once.
			hinted := gridModeBannerText(kind, n, true)
			if hinted != got+i18n.T(i18n.KeyBannerEscHint) {
				t.Fatalf("[%v] %v hinted banner = %q, want %q + esc hint", loc, kind, hinted, got)
			}
		}
		// No shouting caps: the legacy ALL-CAPS wording is retired.
		move := gridModeBannerText(GridModeMove, n, false)
		if strings.Contains(move, "MOVING NODE") {
			t.Fatalf("[%v] move banner still ALL-CAPS: %q", loc, move)
		}
	}

	// EN and ES actually differ (real translations, not copies).
	i18n.SetLocale(i18n.LocaleEN)
	en := gridModeBannerText(GridModeMove, n, false)
	i18n.SetLocale(i18n.LocaleES)
	es := gridModeBannerText(GridModeMove, n, false)
	if en == es {
		t.Fatalf("EN and ES move banners identical: %q", en)
	}

	// GridModeNone renders nothing.
	i18n.SetLocale(i18n.LocaleEN)
	if got := gridModeBannerText(GridModeNone, n, true); got != "" {
		t.Fatalf("none-mode banner = %q, want empty", got)
	}
}
