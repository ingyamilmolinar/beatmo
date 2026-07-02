package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func TestOverflowMenuLabelsLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	g.Layout(1200, 800)
	dv := g.drum
	i18n.SetLocale(i18n.LocaleES)
	labels := map[string]bool{}
	for _, it := range dv.overflowItems() {
		labels[it.label] = true
	}
	for _, want := range []string{"Importar", "Exportar", "Cargar plantilla", "Subir"} {
		if !labels[want] {
			t.Fatalf("overflow menu missing localized label %q; got %v", want, labels)
		}
	}
}

func TestContextMenuLabelsLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	g.Layout(1200, 800)
	dv := g.drum
	i18n.SetLocale(i18n.LocaleES)
	labels := map[string]bool{}
	for _, it := range dv.contextMenuItems(0) {
		labels[it.label] = true
	}
	for _, want := range []string{"Renombrar", "Origen", "Eliminar"} {
		if !labels[want] {
			t.Fatalf("context menu missing localized label %q; got %v", want, labels)
		}
	}
}

// TestInstMenuBreadcrumbLocalized pins the instrument-picker breadcrumb chrome
// ("Categories" / "Favorites") to the central i18n catalog rather than hardcoded
// English literals.
func TestInstMenuBreadcrumbLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:      image.Rect(0, 0, 200, 24),
		VertBounds:      image.Rect(0, 0, 400, 800),
		RowHeight:       24,
		Categories:      []string{"Drums", "Synths"},
		Instruments:     []InstrumentOption{{ID: "kick", Label: "Kick", Category: "Drums"}},
		ForceCategories: true,
	})
	comp.Open()

	i18n.SetLocale(i18n.LocaleES)
	root := comp.BreadcrumbPath()
	if len(root) != 1 || root[0] != "Categorías" {
		t.Fatalf("breadcrumb root not localized: got %v, want [Categorías]", root)
	}

	// Drill into the virtual Favorites view; the trailing segment localizes too.
	comp.state.mode = InstMenuModeInstruments
	comp.state.favoritesView = true
	comp.rebuildMenu()
	path := comp.BreadcrumbPath()
	if len(path) != 2 || path[0] != "Categorías" || path[1] != "Favoritos" {
		t.Fatalf("favorites breadcrumb not localized: got %v, want [Categorías Favoritos]", path)
	}
}

// TestSynthActionLabelsLocalized pins the shared Synth/Sampler action button
// labels (synthActionLabel) to localized, non-empty strings in every supported
// language.
func TestSynthActionLabelsLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	for _, action := range []string{"preview", "save", "save-as", "reset"} {
		if got := synthActionLabel(action); got == "" {
			t.Fatalf("synthActionLabel(%q) resolved empty under es-419", action)
		}
	}
}
