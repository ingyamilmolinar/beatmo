//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func openCategoriesModeMenu(t *testing.T) *InstrumentMenuComponent {
	t.Helper()
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:        image.Rect(10, 100, 100, 124),
		VertBounds:        image.Rect(0, 50, 300, 500),
		CurrentInstrument: roster.Kick,
		Categories:        []audio.Category{audio.CatKick, audio.CatSnare, audio.CatCymbal},
		Instruments: []InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatKick},
			{ID: roster.Snare, Label: "Snare", Category: audio.CatSnare},
			{ID: roster.Hihat, Label: "Hi-Hat", Category: audio.CatCymbal},
		},
		RowHeight:       24,
		ForceCategories: true,
	})
	comp.Open()
	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("expected categories mode, got %s", comp.Mode())
	}
	return comp
}

// Categories mode renders a title band above the rows, as instruments mode renders its breadcrumb, or the
// menu opens as a bare list with the close × on the first row.
func TestInstrumentMenuCategoriesModeHasTitleBand(t *testing.T) {
	comp := openCategoriesModeMenu(t)

	if comp.breadcrumbRect.Empty() {
		t.Fatal("categories mode: breadcrumb/title strip not laid out")
	}
	if !comp.breadcrumbRect.In(comp.Bounds()) {
		t.Fatalf("title strip %v outside menu bounds %v", comp.breadcrumbRect, comp.Bounds())
	}
	for i, btn := range comp.categoryBtns {
		if btn.Rect().Overlaps(comp.breadcrumbRect) {
			t.Fatalf("category row %d rect %v overlaps title strip %v", i, btn.Rect(), comp.breadcrumbRect)
		}
	}
}

// The × shares the title band, which reserves clearance for it.
func TestInstrumentMenuCategoriesModeCloseClearOfRows(t *testing.T) {
	comp := openCategoriesModeMenu(t)

	if comp.closeBtn == nil {
		t.Fatal("categories mode: close button missing")
	}
	for i, btn := range comp.categoryBtns {
		if btn.Rect().Overlaps(comp.closeBtn.Rect()) {
			t.Fatalf("category row %d rect %v overlaps close button %v", i, btn.Rect(), comp.closeBtn.Rect())
		}
	}
}
