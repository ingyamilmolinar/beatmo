package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestFunctional_SearchCursorStaysAfterMidTextInsert reproduces the reported
// bug: place the caret at the START of an existing instrument-search query and
// type a character (e.g. a space) — the character inserts in place, but the
// caret jumps to the END of the text. Root cause: every keystroke changes the
// query → rebuildMenu() → buildInstrumentsMode() unconditionally calls
// searchBox.SetText(state.searchText), and SetText resets the cursor to the
// end even when the text is already identical. The caret must stay right
// after the inserted character.
//
// Drives the real Game.Update loop with the fakeInput harness: type a query,
// click at the left edge of the box to move the caret to position 0, type a
// space, and assert both the text and the caret position.
func TestFunctional_SearchCursorStaysAfterMidTextInsert(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 3; i++ {
		fi.frame(t, g)
	}
	if len(g.drum.Rows) == 0 {
		t.Skip("no rows")
	}
	g.drum.openInstMenuForRow(0)
	fi.frame(t, g)
	m := g.drum.instMenuComp
	if m == nil || !m.IsOpen() {
		t.Skip("instrument menu did not open")
	}
	// Drill into instruments mode if the menu opened on categories.
	if m.Mode() == InstMenuModeCategories {
		cats := m.CategoryBtns()
		if len(cats) == 0 || cats[0].OnClick == nil {
			t.Skip("no category rows to drill into")
		}
		cats[0].OnClick()
		fi.frame(t, g)
	}
	if m.Mode() != InstMenuModeInstruments || m.searchBox == nil {
		t.Skip("instruments mode with search box not reachable")
	}

	// Focus the search box and type a query through the real input path.
	m.searchBox.SetFocus(true)
	fi.chars = []rune("kick")
	fi.frame(t, g)
	if got := m.searchBox.Value(); got != "kick" {
		t.Fatalf("typed query did not land in the search box: %q", got)
	}

	// Click at the very left of the box — the caret moves to position 0.
	r := m.searchBox.Rect
	if r.Empty() {
		t.Fatal("search box has no rect")
	}
	fi.clickAt(t, g, r.Min.X+5, (r.Min.Y+r.Max.Y)/2)
	if !m.searchBox.Focused() {
		t.Fatal("search box lost focus after clicking inside it")
	}
	if m.searchBox.cursor != 0 {
		t.Fatalf("click at left edge should place the caret at 0, got %d", m.searchBox.cursor)
	}

	// Type a space at the start.
	fi.chars = []rune{' '}
	fi.frame(t, g)

	if got := m.searchBox.Value(); got != " kick" {
		t.Fatalf("space should insert at the caret: got %q, want %q", got, " kick")
	}
	if m.searchBox.cursor != 1 {
		t.Fatalf("CURSOR JUMP BUG: after inserting a space at position 0 the caret "+
			"must sit at 1 (right after the space), got %d (end of text) — the search "+
			"rebuild reset the cursor via SetText", m.searchBox.cursor)
	}

	// The live filter must have seen the new query too.
	if m.state.searchText != " kick" {
		t.Fatalf("search state did not follow the box: %q", m.state.searchText)
	}
}

// openInstrumentsSearchDesktop opens the instrument menu on a desktop game,
// drills to instruments mode if needed, focuses the search box, and types the
// given seed query through the real input path. Returns the menu component.
func openInstrumentsSearchDesktop(t *testing.T, g *Game, fi *fakeInput, seed string) *InstrumentMenuComponent {
	t.Helper()
	for i := 0; i < 3; i++ {
		fi.frame(t, g)
	}
	if len(g.drum.Rows) == 0 {
		t.Skip("no rows")
	}
	g.drum.openInstMenuForRow(0)
	fi.frame(t, g)
	m := g.drum.instMenuComp
	if m == nil || !m.IsOpen() {
		t.Skip("instrument menu did not open")
	}
	if m.Mode() == InstMenuModeCategories {
		cats := m.CategoryBtns()
		if len(cats) == 0 || cats[0].OnClick == nil {
			t.Skip("no category rows to drill into")
		}
		cats[0].OnClick()
		fi.frame(t, g)
	}
	if m.Mode() != InstMenuModeInstruments || m.searchBox == nil {
		t.Skip("instruments mode with search box not reachable")
	}
	m.searchBox.SetFocus(true)
	fi.chars = []rune(seed)
	fi.frame(t, g)
	if got := m.searchBox.Value(); got != seed {
		t.Fatalf("seed query did not land in the search box: %q", got)
	}
	return m
}

// pressArrow presses an arrow key n times through the real key-repeat path.
func pressArrow(t *testing.T, g *Game, fi *fakeInput, k ebiten.Key, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		fi.pressKey(t, g, k)
	}
}

// TestFunctional_SearchArrowKeysThenInsertAtStart reproduces the reported bug:
// move the caret to the START of the query with the LEFT ARROW key (not a
// click) and type a character — the character must insert at the caret and
// the caret must advance by one. "Nothing happens" is the bug.
func TestFunctional_SearchArrowKeysThenInsertAtStart(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	m := openInstrumentsSearchDesktop(t, g, fi, "kick")

	pressArrow(t, g, fi, ebiten.KeyLeft, 4)
	if m.searchBox.cursor != 0 {
		t.Fatalf("four Left presses must move the caret from 4 to 0, got %d", m.searchBox.cursor)
	}

	fi.chars = []rune{'x'}
	fi.frame(t, g)

	if got := m.searchBox.Value(); got != "xkick" {
		t.Fatalf("ARROW-INSERT BUG: typing 'x' with the caret at 0 must produce "+
			"%q, got %q (nothing inserted / inserted elsewhere)", "xkick", got)
	}
	if m.searchBox.cursor != 1 {
		t.Fatalf("caret must sit right after the inserted char (1), got %d", m.searchBox.cursor)
	}
	if m.state.searchText != "xkick" {
		t.Fatalf("live filter must follow the edited query, got %q", m.state.searchText)
	}
}

// TestFunctional_SearchArrowKeysMidTextEditing exercises the full caret
// editing contract in the search bar: arrows left/right, insert mid-text, and
// backspace mid-text — all through the real Game.Update loop.
func TestFunctional_SearchArrowKeysMidTextEditing(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	m := openInstrumentsSearchDesktop(t, g, fi, "kick")

	// Left ×2 → caret 2, insert 'z' → "kizck", caret 3.
	pressArrow(t, g, fi, ebiten.KeyLeft, 2)
	fi.chars = []rune{'z'}
	fi.frame(t, g)
	if got := m.searchBox.Value(); got != "kizck" {
		t.Fatalf("mid-text insert: want %q, got %q", "kizck", got)
	}
	if m.searchBox.cursor != 3 {
		t.Fatalf("mid-text insert: caret want 3, got %d", m.searchBox.cursor)
	}

	// Backspace removes the char BEFORE the caret → "kick", caret 2.
	fi.pressKey(t, g, ebiten.KeyBackspace)
	if got := m.searchBox.Value(); got != "kick" {
		t.Fatalf("mid-text backspace: want %q, got %q", "kick", got)
	}
	if m.searchBox.cursor != 2 {
		t.Fatalf("mid-text backspace: caret want 2, got %d", m.searchBox.cursor)
	}

	// Right ×1 → caret 3, insert 'q' → "kicqk", caret 4; filter follows.
	pressArrow(t, g, fi, ebiten.KeyRight, 1)
	fi.chars = []rune{'q'}
	fi.frame(t, g)
	if got := m.searchBox.Value(); got != "kicqk" {
		t.Fatalf("insert after Right: want %q, got %q", "kicqk", got)
	}
	if m.searchBox.cursor != 4 {
		t.Fatalf("insert after Right: caret want 4, got %d", m.searchBox.cursor)
	}
	if m.state.searchText != "kicqk" {
		t.Fatalf("live filter must follow every edit, got %q", m.state.searchText)
	}
}

// TestFunctional_GlobalSearchArrowKeysThenInsertAtStart is the same
// arrow-to-start editing contract on the CATEGORIES-view global search bar.
func TestFunctional_GlobalSearchArrowKeysThenInsertAtStart(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 3; i++ {
		fi.frame(t, g)
	}
	if len(g.drum.Rows) == 0 {
		t.Skip("no rows")
	}
	g.drum.openInstMenuForRow(0)
	fi.frame(t, g)
	m := g.drum.instMenuComp
	if m == nil || !m.IsOpen() {
		t.Skip("instrument menu did not open")
	}
	// Reach the categories view via the production breadcrumb-root path.
	if m.Mode() != InstMenuModeCategories {
		m.popBreadcrumbTo(0)
		fi.frame(t, g)
	}
	if m.Mode() != InstMenuModeCategories || m.searchBox == nil {
		t.Fatal("categories mode with the global search box not reachable")
	}

	m.searchBox.SetFocus(true)
	fi.chars = []rune("kick")
	fi.frame(t, g)
	if got := m.searchBox.Value(); got != "kick" {
		t.Fatalf("seed query did not land in the global search box: %q", got)
	}

	pressArrow(t, g, fi, ebiten.KeyLeft, 4)
	if m.searchBox.cursor != 0 {
		t.Fatalf("four Left presses must move the caret to 0, got %d", m.searchBox.cursor)
	}
	fi.chars = []rune{'x'}
	fi.frame(t, g)
	if got := m.searchBox.Value(); got != "xkick" {
		t.Fatalf("GLOBAL-BAR ARROW-INSERT BUG: typing 'x' with caret at 0 must "+
			"produce %q, got %q", "xkick", got)
	}
	if m.state.searchText != "xkick" {
		t.Fatalf("global filter must follow the edited query, got %q", m.state.searchText)
	}
}
