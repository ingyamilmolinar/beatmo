//go:build test

// The instrument menu's KEYBOARD CURSOR: the border highlight the arrows move,
// and the Shift+Enter ★ shortcut that acts on it.
//
// The menu had arrow support but drew NOTHING for the cursor, so the user was
// steering an invisible selection — arrows moved a highlight they could not
// see. The ring is the same genColorFocusRing outline the note matrix and the
// colour picker already paint for their cursors.
//
// INDEX SPACES (the trap these tests exist to pin): MenuKeyNav addresses
// state.filteredInsts — the FULL filtered list — while instBtns / instIDs /
// favRects are the scrolled VIEWPORT, indexed from scroll.VS.First. Anything
// that maps a cursor to a row must subtract First, or it silently acts on the
// wrong instrument the moment the list scrolls.

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// newKeyCursorMenu builds a standalone instrument menu with `n` instruments and
// a favourites store, opened in instruments mode.
func newKeyCursorMenu(t *testing.T, n int) (*InstrumentMenuComponent, *InMemoryFavoritesStore) {
	t.Helper()
	return newKeyCursorMenuCurrent(t, n, "")
}

// newKeyCursorMenuCurrent is newKeyCursorMenu with props.CurrentInstrument set.
// That single prop changes rebuildMenu's behaviour — it re-biases scroll.VS.First
// onto the current instrument whenever state.userScrolled is false — so a menu
// built without it cannot see scroll bugs that only bite in the real app.
func newKeyCursorMenuCurrent(t *testing.T, n int, current string) (*InstrumentMenuComponent, *InMemoryFavoritesStore) {
	t.Helper()
	store := NewInMemoryFavoritesStore()
	insts := make([]InstrumentOption, n)
	for i := range insts {
		insts[i] = InstrumentOption{
			ID:       instrument.ID("inst" + itoaPad(i)),
			Label:    "Item " + itoaPad(i),
			Category: audio.CatPercussion,
		}
	}
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:        image.Rect(0, 0, 200, 24),
		VertBounds:        image.Rect(0, 0, 400, 400),
		RowHeight:         24,
		Instruments:       insts,
		Favorites:         store,
		CurrentInstrument: instrument.ID(current),
	})
	comp.Open()
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("harness: menu opened in %v, want instruments mode", comp.Mode())
	}
	return comp, store
}

// focusRings returns every unfilled focus-ring draw call the recorder saw.
func focusRings(rec *drawCallRecorder) []drawCall {
	var out []drawCall
	for _, c := range rec.calls {
		if c.Filled {
			continue
		}
		if c.Color == genColorFocusRing {
			out = append(out, c)
		}
	}
	return out
}

func TestInstMenuDrawsFocusRingOnCursorRow(t *testing.T) {
	comp, _ := newKeyCursorMenu(t, 6)
	nav := comp.MenuKeyNavForTest()
	nav.BindHost(comp)
	nav.SetSelected(2)

	screen := ebiten.NewImage(400, 400)
	rec := &drawCallRecorder{}
	rec.record(t, func() { comp.Draw(screen) })

	rings := focusRings(rec)
	if len(rings) == 0 {
		t.Fatal("no focus ring drawn for the keyboard cursor — the arrows move a " +
			"highlight the user cannot see")
	}
	want := comp.InstBtns()[2].Rect()
	for _, ring := range rings {
		if ring.Rect.Overlaps(want) {
			return
		}
	}
	t.Errorf("focus ring drawn at %v, but the cursor row is at %v", rings[0].Rect, want)
}

func TestInstMenuNoFocusRingWhenNothingHighlighted(t *testing.T) {
	comp, _ := newKeyCursorMenu(t, 6)
	if got := comp.MenuKeyNavForTest().Selected(); got != MenuKeyNavNoSelection {
		t.Fatalf("harness: menu opened with row %d highlighted", got)
	}

	screen := ebiten.NewImage(400, 400)
	rec := &drawCallRecorder{}
	rec.record(t, func() { comp.Draw(screen) })

	if rings := focusRings(rec); len(rings) != 0 {
		t.Errorf("a freshly opened menu drew %d focus ring(s) at %v; nothing is "+
			"highlighted yet, so the ring must not imply a default row",
			len(rings), rings[0].Rect)
	}
}

func TestInstMenuFocusRingFollowsTheCursor(t *testing.T) {
	comp, _ := newKeyCursorMenu(t, 6)
	nav := comp.MenuKeyNavForTest()
	nav.BindHost(comp)
	screen := ebiten.NewImage(400, 400)

	ringFor := func(row int) image.Rectangle {
		nav.SetSelected(row)
		rec := &drawCallRecorder{}
		rec.record(t, func() { comp.Draw(screen) })
		rings := focusRings(rec)
		if len(rings) == 0 {
			t.Fatalf("no focus ring drawn with the cursor on row %d", row)
		}
		return rings[0].Rect
	}

	if a, b := ringFor(1), ringFor(3); a == b {
		t.Errorf("the focus ring stayed at %v when the cursor moved from row 1 to 3", a)
	}
}

// TestInstMenuShiftEnterStarsTheCursorRow is the ★ shortcut.
func TestInstMenuShiftEnterStarsTheCursorRow(t *testing.T) {
	ks := installInputStub(t)
	comp, store := newKeyCursorMenu(t, 6)
	nav := comp.MenuKeyNavForTest()
	nav.BindHost(comp)
	nav.SetSelected(2)
	wantID := comp.FilteredInstIDsForTest()[2]

	if store.Get(wantID) {
		t.Fatalf("harness: %q is already a favourite", wantID)
	}

	ks.press(ebiten.KeyShiftLeft)
	ks.press(ebiten.KeyEnter)
	comp.Update()
	ks.release(ebiten.KeyEnter)
	ks.release(ebiten.KeyShiftLeft)
	comp.Update()

	if !store.Get(wantID) {
		t.Errorf("Shift+Enter did not star %q (the cursor row)", wantID)
	}
	if !comp.IsOpen() {
		t.Error("Shift+Enter closed the menu; starring is not a selection")
	}
}

func TestInstMenuShiftEnterUnstarsOnSecondPress(t *testing.T) {
	ks := installInputStub(t)
	comp, store := newKeyCursorMenu(t, 6)
	nav := comp.MenuKeyNavForTest()
	nav.BindHost(comp)
	nav.SetSelected(1)
	id := comp.FilteredInstIDsForTest()[1]

	star := func() {
		ks.press(ebiten.KeyShiftLeft)
		ks.press(ebiten.KeyEnter)
		comp.Update()
		ks.release(ebiten.KeyEnter)
		ks.release(ebiten.KeyShiftLeft)
		comp.Update()
	}
	star()
	if !store.Get(id) {
		t.Fatalf("first Shift+Enter did not star %q", id)
	}
	star()
	if store.Get(id) {
		t.Errorf("second Shift+Enter did not UN-star %q; the ★ is a toggle", id)
	}
}

// TestInstMenuKeyboardScrollRebuildsVisibleRows pins that the DRAWN rows follow
// the cursor when the keyboard scrolls the list.
//
// MenuKeyNav.reveal moves scroll.VS.First so the cursor stays on the visible
// page, but instBtns/instIDs/favRects are only regenerated by rebuildMenu —
// which the keyboard path never triggered. End/PageDown therefore moved the
// scroll state while the list on screen stayed frozen on the first page, so the
// highlight walked off the page and the user steered a list that never moved.
func TestInstMenuKeyboardScrollRebuildsVisibleRows(t *testing.T) {
	ks := installInputStub(t)
	comp, _ := newKeyCursorMenu(t, 60)
	nav := comp.MenuKeyNavForTest()
	nav.BindHost(comp)

	ks.press(ebiten.KeyEnd)
	comp.Update()
	ks.release(ebiten.KeyEnd)
	comp.Update()

	ids := comp.FilteredInstIDsForTest()
	sel := nav.Selected()
	if sel != len(ids)-1 {
		t.Fatalf("harness: End left the cursor at %d, want %d", sel, len(ids)-1)
	}
	wantID := ids[sel]

	// Assert on instIDs — the row-indexed slice drawInstRow and toggleFavoriteAt
	// actually use. NOT VisibleInstIDs, which DERIVES ids from filteredInsts +
	// scroll.First and therefore reports the right answer even while the drawn
	// rows are stale; that divergence is what hid this bug.
	for _, id := range comp.instIDs {
		if id == wantID {
			return
		}
	}
	t.Errorf("after End the cursor is on %q but the DRAWN rows are %v — the list "+
		"did not scroll to follow the cursor (scroll.VS.First moved to %d without "+
		"a rebuildMenu)", wantID, comp.instIDs, comp.scroll.VS.First)
}

// TestInstMenuShiftEnterStarsCorrectRowWhenScrolled is THE index-space test.
//
// MenuKeyNav counts filteredInsts; favRects/instIDs count the scrolled
// viewport. Mapping one onto the other without subtracting scroll.VS.First
// stars whatever instrument happens to sit at that VIEWPORT slot — silently,
// and only once the list is long enough to scroll.
func TestInstMenuShiftEnterStarsCorrectRowWhenScrolled(t *testing.T) {
	ks := installInputStub(t)
	comp, store := newKeyCursorMenu(t, 60)
	nav := comp.MenuKeyNavForTest()
	nav.BindHost(comp)

	// Drive the cursor to the very last row, which forces the list to scroll.
	nav.Update(comp)
	comp.MenuKeyNavForTest().SetSelected(0)
	ks.press(ebiten.KeyEnd)
	comp.Update()
	ks.release(ebiten.KeyEnd)
	comp.Update()

	ids := comp.FilteredInstIDsForTest()
	sel := nav.Selected()
	if sel != len(ids)-1 {
		t.Fatalf("harness: End left the cursor at %d, want the last row %d", sel, len(ids)-1)
	}
	if comp.scroll.VS.First == 0 {
		t.Fatalf("harness: the list did not scroll (First=0), so this test cannot " +
			"distinguish the absolute index from the viewport index")
	}
	wantID := ids[sel]

	ks.press(ebiten.KeyShiftLeft)
	ks.press(ebiten.KeyEnter)
	comp.Update()
	ks.release(ebiten.KeyEnter)
	ks.release(ebiten.KeyShiftLeft)
	comp.Update()

	if !store.Get(wantID) {
		var wrong []string
		for _, id := range ids {
			if store.Get(id) {
				wrong = append(wrong, string(id))
			}
		}
		t.Errorf("Shift+Enter on the cursor row starred %v, want %q — the cursor "+
			"indexes filteredInsts but favRects indexes the scrolled viewport "+
			"(subtract scroll.VS.First)", wrong, wantID)
	}
}

// TestInstMenuFocusRingStaysVisibleWhileArrowingPastThePage walks the cursor
// down past the bottom of the visible page with repeated ArrowDown — the
// gesture the feature exists for — and asserts the ring is on screen at every
// step, on the row the cursor actually points at.
//
// This is the end-to-end version of the scroll/rebuild pair: reveal() scrolls,
// the rebuild refreshes the rows, and keyCursorViewportRow rebases the cursor.
// Break any one of the three and the ring either vanishes or lands on the wrong
// row once the list starts scrolling.
func TestInstMenuFocusRingStaysVisibleWhileArrowingPastThePage(t *testing.T) {
	ks := installInputStub(t)
	comp, _ := newKeyCursorMenu(t, 40)
	nav := comp.MenuKeyNavForTest()
	nav.BindHost(comp)
	screen := ebiten.NewImage(400, 400)

	visible := comp.scroll.VS.Visible
	if visible <= 0 || visible >= 40 {
		t.Fatalf("harness: need a scrolling list, got Visible=%d of 40", visible)
	}

	for step := 0; step < visible+5; step++ {
		ks.press(ebiten.KeyArrowDown)
		comp.Update()
		ks.release(ebiten.KeyArrowDown)
		comp.Update()

		sel := nav.Selected()
		if sel != step {
			t.Fatalf("step %d: cursor at %d, want %d", step, sel, step)
		}

		rec := &drawCallRecorder{}
		rec.record(t, func() { comp.Draw(screen) })
		rings := focusRings(rec)
		if len(rings) == 0 {
			t.Fatalf("step %d (cursor=%d, First=%d): the focus ring vanished — the "+
				"user is steering an invisible highlight", step, sel, comp.scroll.VS.First)
		}

		// The ring must sit on the row the cursor names, not merely somewhere.
		wantID := comp.FilteredInstIDsForTest()[sel]
		row := sel - comp.scroll.VS.First
		if row < 0 || row >= len(comp.instIDs) {
			t.Fatalf("step %d: cursor %d is outside the drawn viewport (First=%d, rows=%d)",
				step, sel, comp.scroll.VS.First, len(comp.instIDs))
		}
		if comp.instIDs[row] != wantID {
			t.Fatalf("step %d: the ring is on row %d which draws %q, but the cursor "+
				"points at %q", step, row, comp.instIDs[row], wantID)
		}
		if got := comp.InstBtns()[row].Rect(); !rings[0].Rect.Overlaps(got) {
			t.Fatalf("step %d: ring at %v, cursor row at %v", step, rings[0].Rect, got)
		}
	}
}

// TestInstMenuKeyboardScrollSurvivesTheRebuild is the companion to
// TestInstMenuKeyboardScrollRebuildsVisibleRows, and the reason that test alone
// is not enough.
//
// rebuildMenu re-biases scroll.VS.First onto props.CurrentInstrument whenever
// state.userScrolled is false. So "scroll with the keyboard, then rebuild to
// refresh the rows" silently SNAPS BACK to the current instrument the moment a
// real CurrentInstrument is set — which is always, in the app. A harness that
// leaves CurrentInstrument empty never sees it.
func TestInstMenuKeyboardScrollSurvivesTheRebuild(t *testing.T) {
	ks := installInputStub(t)
	comp, _ := newKeyCursorMenuCurrent(t, 60, "inst000")
	nav := comp.MenuKeyNavForTest()
	nav.BindHost(comp)

	ks.press(ebiten.KeyEnd)
	comp.Update()
	ks.release(ebiten.KeyEnd)
	comp.Update()

	ids := comp.FilteredInstIDsForTest()
	sel := nav.Selected()
	if sel != len(ids)-1 {
		t.Fatalf("harness: End left the cursor at %d, want %d", sel, len(ids)-1)
	}
	wantID := ids[sel]

	for _, id := range comp.instIDs {
		if id == wantID {
			return
		}
	}
	t.Errorf("after End the cursor is on %q but the drawn rows are %v (First=%d) — "+
		"the rebuild re-biased the scroll back onto CurrentInstrument. A keyboard "+
		"scroll is a USER scroll and must set state.userScrolled.",
		wantID, comp.instIDs, comp.scroll.VS.First)
}

// TestShiftEnterPutsTheInstrumentInTheFavoritesCategory is the user-visible end
// of the ★ shortcut: not "the store flag flipped" but "the instrument is now
// listed under Favorites".
//
// Asserting on the store alone would pass even if the Favorites view read from
// a different source — the two are only connected because favoritesIDs consults
// props.Favorites, and nothing in the compiler enforces that.
func TestShiftEnterPutsTheInstrumentInTheFavoritesCategory(t *testing.T) {
	ks := installInputStub(t)
	comp, store := newKeyCursorMenu(t, 6)
	nav := comp.MenuKeyNavForTest()
	nav.BindHost(comp)
	nav.SetSelected(3)
	wantID := comp.FilteredInstIDsForTest()[3]

	ks.press(ebiten.KeyShiftLeft)
	ks.press(ebiten.KeyEnter)
	comp.Update()
	ks.release(ebiten.KeyEnter)
	ks.release(ebiten.KeyShiftLeft)
	comp.Update()

	if !store.Get(wantID) {
		t.Fatalf("Shift+Enter did not star %q", wantID)
	}

	// Switch the menu into the virtual Favorites view the ★ category opens.
	comp.state.favoritesView = true
	comp.state.activeCat = audio.CatNone
	comp.rebuildMenu()

	listed := comp.FilteredInstIDsForTest()
	for _, id := range listed {
		if id == wantID {
			return
		}
	}
	t.Errorf("Shift+Enter starred %q but the Favorites view lists %v — starring "+
		"did not put the instrument in the Favorites category", wantID, listed)
}
