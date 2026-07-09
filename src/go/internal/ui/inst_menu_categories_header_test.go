//go:build test

package ui

import (
	"image"
	"testing"
)

func openCategoriesModeMenu(t *testing.T) *InstrumentMenuComponent {
	t.Helper()
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:        image.Rect(10, 100, 100, 124),
		VertBounds:        image.Rect(0, 50, 300, 500),
		CurrentInstrument: "kick",
		Categories:        []string{"Kicks", "Snares", "Cymbals"},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: "Kicks"},
			{ID: "snare", Label: "Snare", Category: "Snares"},
			{ID: "hihat", Label: "Hi-Hat", Category: "Cymbals"},
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

// Categories mode must render a title band (the "Categories" breadcrumb
// root) above the rows, exactly like instruments mode renders its
// breadcrumb strip. Regression: pre-fix, categories mode laid out rows
// from the panel's very top — no title at all — so the menu opened as a
// bare list and the close × sat on top of the first row.
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

// The close × must never overlap a category row — it shares the title
// band, which reserves clearance for it (mirrors instruments mode).
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
