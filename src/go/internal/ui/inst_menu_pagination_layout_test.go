//go:build test

package ui

import (
	"fmt"
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Everything the menu lays out lives inside its own bounds: a long list scrolls, and no chrome may sit below
// fullRect, where it floats detached on desktop and is clipped off the bottom sheet on mobile.
func TestInstrumentMenuLongListStaysInsidePanel(t *testing.T) {
	comp := NewInstrumentMenuComponent()

	// 25 instruments in one category: more than one page at 10 visible rows.
	var insts []InstrumentOption
	for i := 0; i < 25; i++ {
		id := fmt.Sprintf("kick-%02d", i)
		insts = append(insts, InstrumentOption{ID: instrument.ID(id), Label: id, Category: audio.CatKick})
	}
	vertBounds := image.Rect(0, 50, 300, 500)
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:        image.Rect(10, 100, 100, 124),
		VertBounds:        vertBounds,
		CurrentInstrument: "kick-00",
		Categories:        []audio.Category{audio.CatKick},
		Instruments:       insts,
		RowHeight:         24,
	})
	comp.Open()

	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode, got %s", comp.Mode())
	}
	if !comp.scroll.HasScroll() {
		t.Fatal("long list must scroll")
	}
	bounds := comp.Bounds()
	if !comp.searchRect.In(bounds) {
		t.Fatalf("search row %v outside panel %v", comp.searchRect, bounds)
	}
	if !comp.scroll.VS.View.In(bounds) {
		t.Fatalf("list view %v outside panel %v", comp.scroll.VS.View, bounds)
	}
	for i, b := range comp.InstBtns() {
		if !b.Rect().In(bounds) {
			t.Fatalf("row %d rect %v outside panel %v", i, b.Rect(), bounds)
		}
	}
	// And the whole panel must respect VertBounds.
	if !bounds.In(vertBounds) {
		t.Fatalf("menu panel %v escapes VertBounds %v", bounds, vertBounds)
	}
}
