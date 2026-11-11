//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A card lays its dials out in rows of as many cells as fit at the minimum
// cell width (at most four): a desktop card holds three, a phone-wide card
// four, so a four-dial card stays one row on a phone. A card too narrow for
// several cells uses fewer per row (more rows) instead of drawing no dials at
// all. Every rect is non-empty, inside the card, and no two overlap. Spec
// 2026-10-13 §3.3 (+ ruling: up to four per row).
func TestDialsLayOutInRowsOfThreeWithoutOverlap(t *testing.T) {
	// A desktop root card (~236 px) holds three per row: four dials → two rows.
	if got := macroDialRows(4, 236); got != 2 {
		t.Errorf("a desktop card lays four dials in %d rows, want 2 (three per row)", got)
	}
	if got := macroDialRows(6, 236); got != 2 {
		t.Errorf("a desktop card lays six dials in %d rows, want 2", got)
	}
	// A phone-wide card (mobile density, ~370 px) holds four: a four-dial card
	// keeps the single-row height, so a phone still shows two cards.
	func() {
		restore := SetDensityForTest(DensitySpacious)
		defer restore()
		if got := macroDialRows(4, 370); got != 1 {
			t.Errorf("a phone-wide card lays four dials in %d rows, want 1", got)
		}
	}()
	for _, w := range []int{140, 220, 300, 390} {
		for n := 1; n <= 6; n++ {
			rows := macroDialRows(n, w)
			card := image.Rect(10, 20, 10+w, 20+macroRootCardH(rows))
			rects := macroCardDialRects(card, n)
			var got []image.Rectangle
			for i := 0; i < n; i++ {
				r := rects[i]
				if r.Empty() {
					t.Fatalf("w=%d n=%d: dial %d has no rect", w, n, i)
				}
				if !r.In(card) {
					t.Errorf("w=%d n=%d: dial %d rect %v leaves the card %v", w, n, i, r, card)
				}
				for j, o := range got {
					if r.Overlaps(o) {
						t.Errorf("w=%d n=%d: dials %d and %d overlap (%v, %v)", w, n, j, i, o, r)
					}
				}
				got = append(got, r)
			}
			per := macroDialsPerRow(n, w)
			if per < 1 || per > 4 || rows*per < n || (rows-1)*per >= n {
				t.Errorf("w=%d n=%d: %d per row × %d rows does not fit n tightly", w, n, per, rows)
			}
			if minW := macroDialCellMinW(); per > 1 && rects[0].Dx() < minW {
				t.Errorf("w=%d n=%d: cells %d px wide, under the %d px minimum", w, n, rects[0].Dx(), minW)
			}
		}
	}
}

// Scrolling the root must never change a card's height or cost a card its
// trailing dials: the cell height comes from the grid's cell width, not from
// the first cell's rect (which is not visible once the root scrolls). Before,
// at some desktop widths a four-dial card scrolled into view with its fourth
// dial undrawn and untappable.
func TestScrolledRootKeepsEveryDial(t *testing.T) {
	scrolled, seen := 0, 0
	defer func() {
		if scrolled == 0 || seen == 0 {
			t.Errorf("no width scrolled row 0 out with the Voice card in view (scrolled %d, seen %d) — the test proves nothing", scrolled, seen)
		}
	}()
	for w := 900; w <= 1600; w += 6 {
		g := newViolinMacroGame(t)
		g.drum.Rows[0].Instrument = roster.VoiceAlien // its five-dial Voice card is the last card
		g.Layout(w, 720)
		g = openDialRoot(t, g)
		dv := g.drum
		grid := dv.synp().sectionGrid(-1)
		grid.ScrollToIndex(len(dv.instEditorMacros) - 1)
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
		if _, ok := grid.CellRect(0); ok {
			continue // row 0 still visible: not the case under test at this width
		}
		scrolled++
		for _, c := range dv.instEditorMacroCards {
			if c.id == "voice" {
				seen++
			}
			for j := 0; j < c.nDials; j++ {
				if c.dialRects[j].Empty() {
					t.Fatalf("window %d px, root scrolled: card %q lost dial %d of %d", w, c.id, j+1, c.nDials)
				}
			}
		}
		g.CloseForTest()
	}
}
