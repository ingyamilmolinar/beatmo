package ui

import (
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
