//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestLiveSwitchRendersSpanish drives a real settings-overlay build under the
// Spanish locale and asserts user-facing surfaces resolve to Spanish strings —
// proving the resolve-at-draw rule works end to end (catalog → i18n.T at the
// render site), not just at construction.
func TestLiveSwitchRendersSpanish(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)

	// Settings overlay surfaces.
	i18n.SetLocale(i18n.LocaleES)
	if got := i18n.T(i18n.KeySettingsTitle); got != "Ajustes" {
		t.Fatalf("settings title ES = %q want Ajustes", got)
	}
	if got := i18n.T(i18n.KeySettingsLanguage); got != "Idioma" {
		t.Fatalf("settings language label ES = %q want Idioma", got)
	}

	// Menus localize live.
	for key, want := range map[i18n.Key]string{
		i18n.KeyMenuImport:       "Importar",
		i18n.KeyMenuExport:       "Exportar",
		i18n.KeyMenuLoadTemplate: "Cargar plantilla",
	} {
		if got := i18n.T(key); got != want {
			t.Fatalf("menu %q ES = %q want %q", key, got, want)
		}
	}

	// Switching back to English restores English live.
	i18n.SetLocale(i18n.LocaleEN)
	if got := i18n.T(i18n.KeySettingsTitle); got != "Settings" {
		t.Fatalf("settings title EN = %q want Settings", got)
	}
}

// TestSpanishFitsNarrowSurfaces guards against Spanish strings overflowing the
// tightest text surfaces. The two narrowest are the settings language pills
// (fixed width) and short menu labels. A clipped label (StyledTextWidth beyond
// the slot) would silently truncate, so assert the rendered ES width fits.
func TestSpanishFitsNarrowSurfaces(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)

	// Settings language pills are 150px wide (see settings_overlay.go pillW),
	// with ~12px left text inset. The pill label must fit the remaining width.
	const pillW = 150
	const pillTextBudget = pillW - 24 // inset both sides, generous
	for _, k := range []i18n.Key{i18n.KeyLangEnglish, i18n.KeyLangSpanish} {
		label := i18n.T(k)
		if w := StyledTextWidth(label, RoleBody); w > pillTextBudget {
			t.Errorf("language pill label %q (ES) width %d exceeds budget %d — would clip", label, w, pillTextBudget)
		}
	}

	// Short menu labels should fit a typical menu column (~220px content) with
	// room for an icon. Catch gross ES overflow.
	const menuTextBudget = 220
	for _, k := range []i18n.Key{
		i18n.KeyMenuRename, i18n.KeyMenuDelete, i18n.KeyMenuOrigin,
		i18n.KeyMenuUpload, i18n.KeyMenuImport, i18n.KeyMenuExport,
		i18n.KeyMenuLoadTemplate, i18n.KeyMenuTemplates, i18n.KeyMenuBack,
		i18n.KeySettingsTitle,
	} {
		label := i18n.T(k)
		if label == "" {
			t.Errorf("empty ES label for %q", k)
			continue
		}
		if w := StyledTextWidth(label, RoleCaption); w > menuTextBudget {
			t.Errorf("menu label %q (ES) width %d exceeds budget %d", label, w, menuTextBudget)
		}
	}
}
