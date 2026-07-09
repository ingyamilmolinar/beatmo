//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Every IconID has exactly one semantic role (DESIGN.md Icon Map). "Load
// template" and the template rows reused IconRows — the "switch to row
// view" glyph — so the same three-lines icon meant two unrelated things.
// They carry the dedicated template-card glyph now.
func TestIconTemplateHasBody(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	if !drawIconByID(dst, IconTemplate, image.Rect(0, 0, 24, 24), color.White) {
		t.Fatal("IconTemplate is not registered in drawIconByID")
	}
}

func TestLoadTemplateUsesTemplateIcon(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	dv := g.drum

	dv.overflowPage = 0
	found := false
	for _, it := range dv.overflowItems() {
		if it.label == i18n.T(i18n.KeyMenuLoadTemplate) {
			found = true
			if it.iconID != IconTemplate {
				t.Fatalf("Load template icon = %q, want IconTemplate (IconRows means 'row view')", it.iconID)
			}
		}
	}
	if !found {
		t.Fatal("Load template entry not found in the File menu")
	}

	dv.overflowPage = 1
	for _, it := range dv.overflowItems() {
		if it.header || it.iconID == IconChevronLeft {
			continue
		}
		if it.iconID != IconTemplate {
			t.Fatalf("template row %q icon = %q, want IconTemplate", it.label, it.iconID)
		}
	}
}
