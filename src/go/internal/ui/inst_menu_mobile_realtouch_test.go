//go:build test

package ui

import (
	"testing"
)

// openMenuInstrumentsRealTouch opens the instrument menu for a row and reaches
// instruments mode (entering the first category if it opened on categories).
func openMenuInstrumentsRealTouch(t *testing.T, g *Game, row int) *InstrumentMenuComponent {
	t.Helper()
	dv := g.drum
	dv.openInstMenuForRow(row)
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}
	comp := dv.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instrument menu should be open")
	}
	if comp.Mode() == InstMenuModeCategories {
		cats := comp.CategoryBtns()
		if len(cats) > 0 && cats[0].OnClick != nil {
			cats[0].OnClick()
		}
		for i := 0; i < 3; i++ {
			_ = g.Update()
		}
	}
	return comp
}

func newMobileGame(t *testing.T) *Game {
	t.Helper()
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	resetTouchOverride()
	t.Cleanup(resetTouchOverride)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	return g
}

// TestInstMenuMobileCategoryForwardRealTouch: in categories mode, a REAL touch
// tap on a category button must navigate into that category (instruments mode).
func TestInstMenuMobileCategoryForwardRealTouch(t *testing.T) {
	g := newMobileGame(t)
	comp := openMenuInstrumentsRealTouch(t, g, 0)
	// Go back to categories (via the real handler), then forward via real touch.
	if comp.Mode() == InstMenuModeInstruments {
		if comp.backBtn != nil && comp.backBtn.OnClick != nil {
			comp.backBtn.OnClick()
		}
		for i := 0; i < 3; i++ {
			_ = g.Update()
		}
	}
	if comp.Mode() != InstMenuModeCategories {
		t.Skipf("could not reach categories mode (got %v)", comp.Mode())
	}
	cats := comp.CategoryBtns()
	if len(cats) == 0 {
		t.Fatal("no category buttons")
	}
	r := cats[0].Rect()
	realTouchTapGame(t, g, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, 2)
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("CATEGORY FORWARD BUG: real-touch tap on a category did not enter instruments mode (mode=%v)", comp.Mode())
	}
}

// TestInstMenuMobileCloseRealTouch: a REAL touch tap on the × must close the menu.
func TestInstMenuMobileCloseRealTouch(t *testing.T) {
	g := newMobileGame(t)
	comp := openMenuInstrumentsRealTouch(t, g, 0)
	cb := comp.CloseBtn()
	if cb == nil || cb.Rect().Empty() {
		t.Skip("no close button rect")
	}
	r := cb.Rect()
	realTouchTapGame(t, g, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, 2)
	if comp.IsOpen() {
		t.Fatalf("CLOSE BUG: real-touch tap on × did not close the instrument menu")
	}
}

// TestInstMenuMobileCrossRowSelectRealTouch: opening the menu for row N and
// selecting must change row N (not row 0).
func TestInstMenuMobileCrossRowSelectRealTouch(t *testing.T) {
	g := newMobileGame(t)
	dv := g.drum
	if len(dv.Rows) < 3 {
		t.Skip("need >=3 rows")
	}
	const row = 2
	row0Before := dv.Rows[0].Instrument
	comp := openMenuInstrumentsRealTouch(t, g, row)
	if comp.Mode() != InstMenuModeInstruments || len(comp.InstBtns()) == 0 {
		t.Fatalf("expected instruments mode with items, mode=%v", comp.Mode())
	}
	cur := dv.Rows[row].Instrument
	idx, wantID := instMenuFindAlternateInst(comp, cur)
	if idx < 0 {
		t.Fatalf("no alternate instrument; ids=%v", comp.VisibleInstIDs())
	}
	r := comp.InstBtns()[idx].Rect()
	realTouchTapGame(t, g, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, 2)
	if dv.Rows[row].Instrument != wantID {
		t.Fatalf("CROSS-ROW BUG: selected for row %d but row %d=%q (want %q)", row, row, dv.Rows[row].Instrument, wantID)
	}
	if dv.Rows[0].Instrument != row0Before {
		t.Fatalf("CROSS-ROW BUG: selecting for row %d changed row 0 (%q -> %q)", row, row0Before, dv.Rows[0].Instrument)
	}
}

// TestInstMenuMobileFavoriteStarRealTouch: a REAL touch tap on a ★ star hit area
// must toggle that instrument's favorite flag.
func TestInstMenuMobileFavoriteStarRealTouch(t *testing.T) {
	g := newMobileGame(t)
	comp := openMenuInstrumentsRealTouch(t, g, 0)
	if len(comp.favRects) == 0 {
		t.Skip("no favorite-star hit areas active in this mode")
	}
	id := comp.favIDs[0]
	_, before := comp.favoritesIDs()[id]
	r := comp.favRects[0]
	realTouchTapGame(t, g, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, 2)
	_, after := comp.favoritesIDs()[id]
	if after == before {
		t.Fatalf("FAVORITE STAR BUG: real-touch tap on ★ for %q did not toggle favorite (stayed %v)", id, before)
	}
}

// realTouchDragGame drives a single-finger DRAG through the full Game.Update
// loop using the mock touch state (NOT SetInputForTest), so the touch override
// + gesture/scroll path are exercised for real. Finger goes down at p0, moves
// through pts over one frame each, then lifts.
func realTouchDragGame(t *testing.T, g *Game, pts [][2]int) {
	t.Helper()
	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	mock.addTouch(1, pts[0][0], pts[0][1])
	_ = g.Update()
	_ = g.Update()
	for _, p := range pts[1:] {
		mock.moveTouch(1, p[0], p[1])
		_ = g.Update()
	}
	mock.removeTouch(1)
	for i := 0; i < 6; i++ {
		_ = g.Update()
	}
}

// TestInstMenuMobileScrolledSelectRealTouch is the scroll-then-select repro:
// scroll a long instrument list via REAL touch, then tap a visible row, and
// assert the instrument that is VISUALLY at that row (filteredInsts[first+row])
// gets selected — not the pre-scroll item, and not nothing.
func TestInstMenuMobileScrolledSelectRealTouch(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	resetTouchOverride()
	t.Cleanup(resetTouchOverride)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	// Short height so even a modest category overflows the bottom-sheet and the
	// list must scroll (mirrors a small phone / landscape).
	const W, H = 390, 430
	g.Layout(W, H)
	dv := g.drum

	dv.openInstMenuForRow(0)
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}
	comp := dv.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instrument menu should be open")
	}
	if comp.Mode() == InstMenuModeCategories {
		cats := comp.CategoryBtns()
		if len(cats) == 0 {
			t.Fatal("no category buttons")
		}
		if cats[0].OnClick != nil {
			cats[0].OnClick()
		}
		for i := 0; i < 3; i++ {
			_ = g.Update()
		}
	}
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode, got %v", comp.Mode())
	}

	first, visible, total := comp.ScrollState()
	t.Logf("scroll state: first=%d visible=%d total=%d filtered=%d", first, visible, total, len(comp.state.filteredInsts))
	if total <= visible {
		t.Skipf("list not scrollable at %dx%d (total=%d visible=%d) — adjust height", W, H, total, visible)
	}

	// Real-touch drag UP inside the list viewport to scroll DOWN.
	view := comp.scroll.VS.View
	cx := (view.Min.X + view.Max.X) / 2
	yTop := view.Min.Y + (view.Dy() / 6)
	yBot := view.Max.Y - (view.Dy() / 6)
	realTouchDragGame(t, g, [][2]int{{cx, yBot}, {cx, yBot - 20}, {cx, (yBot + yTop) / 2}, {cx, yTop + 20}, {cx, yTop}})
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}

	first2, _, _ := comp.ScrollState()
	t.Logf("after drag: first=%d", first2)
	if first2 == 0 {
		t.Skipf("drag did not commit a scroll (first still 0) — drag calibration")
	}

	// Tap visible row 0 — the item VISUALLY there is filteredInsts[first2].
	btns := comp.InstBtns()
	ids := comp.VisibleInstIDs()
	if len(btns) == 0 {
		t.Fatal("no instrument buttons after scroll")
	}
	row := 0
	wantID := comp.state.filteredInsts[first2+row]
	// Sanity: the exported visible id at that row should already equal wantID.
	if ids[row] != wantID {
		t.Errorf("CONSISTENCY: VisibleInstIDs[%d]=%q but filteredInsts[first+%d]=%q (rect/id desync)", row, ids[row], row, wantID)
	}
	r := btns[row].Rect()
	tx := r.Min.X + r.Dx()/2
	ty := r.Min.Y + r.Dy()/2

	before := dv.Rows[0].Instrument
	realTouchTapGame(t, g, tx, ty, 2)

	got := dv.Rows[0].Instrument
	t.Logf("scrolled select: before=%q tapped row %d (rect y=%d) wantVisible=%q got=%q", before, row, ty, wantID, got)
	if got != wantID {
		t.Fatalf("SCROLLED SELECT BUG: tapped visible row %d (showing %q) but selected %q",
			row, wantID, got)
	}
}
