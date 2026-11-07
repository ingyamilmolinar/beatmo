//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// As many cells per row as fit (≤ 4): desktop cards hold three, phone cards four; too narrow means more rows, never
// no dials. Every rect non-empty, inside its card, non-overlapping.
func TestDialsLayOutInRowsOfThreeWithoutOverlap(t *testing.T) {
	// A desktop root card (~236 px) holds three per row: four dials → two rows.
	if got := macroDialRows(4, 236); got != 2 {
		t.Errorf("a desktop card lays four dials in %d rows, want 2 (three per row)", got)
	}
	if got := macroDialRows(6, 236); got != 2 {
		t.Errorf("a desktop card lays six dials in %d rows, want 2", got)
	}
	// Four on a ~370 px phone card, so a four-dial card stays one row and two cards still show.
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

// The cell height comes from the grid's cell width, not the first (scrolled-off) cell, so a card keeps its height
// and every dial when the root scrolls.
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
