//go:build test

package ui

import (
	"fmt"
	"image"
	"testing"
)

// A long instrument list must stay fully inside the menu panel and rely on
// scrolling — no auxiliary chrome may lay out below fullRect. Regression:
// the retired pagination strip was laid out at fullRect.Max.Y — entirely
// outside the drawn panel — so it rendered as a detached floating row
// (desktop) or was clipped off the bottom screen edge (mobile bottom
// sheet). The strip is gone; this pins the invariant that killing it
// restored: everything the menu lays out lives inside its own bounds.
func TestInstrumentMenuLongListStaysInsidePanel(t *testing.T) {
	comp := NewInstrumentMenuComponent()

	// 25 instruments in one category: more than one page at 10 visible rows.
	var insts []InstrumentOption
	for i := 0; i < 25; i++ {
		id := fmt.Sprintf("kick-%02d", i)
		insts = append(insts, InstrumentOption{ID: id, Label: id, Category: "Kicks"})
	}
	vertBounds := image.Rect(0, 50, 300, 500)
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:        image.Rect(10, 100, 100, 124),
		VertBounds:        vertBounds,
		CurrentInstrument: "kick-00",
		Categories:        []string{"Kicks"},
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
