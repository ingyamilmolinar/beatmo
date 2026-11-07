//go:build test

// The new capability is MenuItemSelectable: Templates interleaves headers and a pinned search row, so the next
// item is not the next index.

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// fakeNavHost is a menu stand-in: rows marked false are headers/separators.
type fakeNavHost struct {
	selectable []bool
	activated  []int
	scroll     *MenuScroll
}

func (h *fakeNavHost) MenuItemCount() int            { return len(h.selectable) }
func (h *fakeNavHost) MenuItemSelectable(i int) bool { return h.selectable[i] }
func (h *fakeNavHost) MenuActivate(i int)            { h.activated = append(h.activated, i) }
func (h *fakeNavHost) MenuScrollBehavior() *ScrollBehavior {
	if h.scroll == nil {
		return nil
	}
	return h.scroll.ScrollBehavior()
}

// Edge-detected: a press is down then up.
func navKeys(t *testing.T, nav *MenuKeyNav, host MenuKeyNavHost) func(ebiten.Key) {
	t.Helper()
	var held ebiten.Key = -1
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == held },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	t.Cleanup(restore)
	return func(k ebiten.Key) {
		held = k
		nav.Update(host) // rising edge -> acts
		held = -1
		nav.Update(host) // release
	}
}

func allSelectable(n int) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = true
	}
	return out
}

func TestMenuKeyNav_StartsWithNothingSelected(t *testing.T) {
	nav := NewMenuKeyNav()
	if got := nav.Selected(); got != -1 {
		t.Errorf("a freshly opened menu has Selected()=%d, want -1 — nothing should be "+
			"highlighted until the user arrows or hovers", got)
	}
}

func TestMenuKeyNav_DownFromNothingSelectsFirst(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(3)}
	nav := NewMenuKeyNav()
	press := navKeys(t, nav, host)
	press(ebiten.KeyArrowDown)
	if got := nav.Selected(); got != 0 {
		t.Errorf("first Down selected %d, want 0", got)
	}
}

func TestMenuKeyNav_UpFromNothingSelectsLast(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(3)}
	nav := NewMenuKeyNav()
	press := navKeys(t, nav, host)
	press(ebiten.KeyArrowUp)
	if got := nav.Selected(); got != 2 {
		t.Errorf("first Up selected %d, want the last row 2", got)
	}
}

func TestMenuKeyNav_SkipsUnselectableRows(t *testing.T) {
	// header, item, header, item, item  — mirrors the Templates menu shape.
	host := &fakeNavHost{selectable: []bool{false, true, false, true, true}}
	nav := NewMenuKeyNav()
	press := navKeys(t, nav, host)

	press(ebiten.KeyArrowDown)
	if got := nav.Selected(); got != 1 {
		t.Fatalf("Down landed on %d, want 1 (index 0 is a group header)", got)
	}
	press(ebiten.KeyArrowDown)
	if got := nav.Selected(); got != 3 {
		t.Fatalf("Down landed on %d, want 3 (index 2 is a group header)", got)
	}
	press(ebiten.KeyArrowUp)
	if got := nav.Selected(); got != 1 {
		t.Fatalf("Up landed on %d, want 1 (skipping the header at 2)", got)
	}
}

func TestMenuKeyNav_ClampsAtBothEnds(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(3)}
	nav := NewMenuKeyNav()
	press := navKeys(t, nav, host)

	for i := 0; i < 6; i++ {
		press(ebiten.KeyArrowDown)
	}
	if got := nav.Selected(); got != 2 {
		t.Errorf("holding Down past the end selected %d, want to stop at 2 (no wrap)", got)
	}
	for i := 0; i < 6; i++ {
		press(ebiten.KeyArrowUp)
	}
	if got := nav.Selected(); got != 0 {
		t.Errorf("holding Up past the start selected %d, want to stop at 0 (no wrap)", got)
	}
}

func TestMenuKeyNav_EnterActivatesSelection(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(3)}
	nav := NewMenuKeyNav()
	press := navKeys(t, nav, host)

	press(ebiten.KeyArrowDown)
	press(ebiten.KeyArrowDown)
	press(ebiten.KeyEnter)
	if len(host.activated) != 1 || host.activated[0] != 1 {
		t.Errorf("Enter activated %v, want exactly [1]", host.activated)
	}
}

func TestMenuKeyNav_EnterWithNoSelectionDoesNothing(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(3)}
	nav := NewMenuKeyNav()
	press := navKeys(t, nav, host)

	press(ebiten.KeyEnter)
	if len(host.activated) != 0 {
		t.Errorf("Enter with nothing highlighted activated %v, want no action — "+
			"otherwise opening a menu and pressing Enter fires an arbitrary row", host.activated)
	}
}

func TestMenuKeyNav_HeldKeyMovesOncePerPress(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(5)}
	nav := NewMenuKeyNav()
	var held ebiten.Key = ebiten.KeyArrowDown
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == held },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	for i := 0; i < 10; i++ { // key held down for ten frames
		nav.Update(host)
	}
	if got := nav.Selected(); got != 0 {
		t.Errorf("holding Down for 10 frames moved to %d, want 0 — movement must be "+
			"rising-edge so a held key does not race through the list", got)
	}
}

func TestMenuKeyNav_HomeAndEnd(t *testing.T) {
	host := &fakeNavHost{selectable: []bool{false, true, true, true, false}}
	nav := NewMenuKeyNav()
	press := navKeys(t, nav, host)

	press(ebiten.KeyEnd)
	if got := nav.Selected(); got != 3 {
		t.Errorf("End selected %d, want the last SELECTABLE row 3 (4 is a header)", got)
	}
	press(ebiten.KeyHome)
	if got := nav.Selected(); got != 1 {
		t.Errorf("Home selected %d, want the first SELECTABLE row 1 (0 is a header)", got)
	}
}

func TestMenuKeyNav_HoverTakesOverTheCursor(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(8)}
	nav := NewMenuKeyNav()
	press := navKeys(t, nav, host)

	press(ebiten.KeyArrowDown)
	press(ebiten.KeyArrowDown) // keyboard cursor on row 1
	nav.SetSelected(6)         // mouse moves over row 6
	if got := nav.Selected(); got != 6 {
		t.Fatalf("after hovering row 6 the cursor is %d, want 6 — there is exactly ONE "+
			"highlight and the mouse takes it over", got)
	}
	press(ebiten.KeyEnter)
	if len(host.activated) != 1 || host.activated[0] != 6 {
		t.Errorf("Enter activated %v, want [6] — Enter acts on the unified highlight", host.activated)
	}
}

func TestMenuKeyNav_SetSelectedRejectsUnselectable(t *testing.T) {
	host := &fakeNavHost{selectable: []bool{true, false, true}}
	nav := NewMenuKeyNav()
	nav.BindHost(host)
	nav.SetSelected(0)
	nav.SetSelected(1) // a header — must not become the cursor
	if got := nav.Selected(); got != 0 {
		t.Errorf("hovering a group header moved the cursor to %d, want it to stay at 0", got)
	}
}

func TestMenuKeyNav_ResetClearsSelection(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(3)}
	nav := NewMenuKeyNav()
	press := navKeys(t, nav, host)
	press(ebiten.KeyArrowDown)
	nav.Reset()
	if got := nav.Selected(); got != -1 {
		t.Errorf("after Reset, Selected()=%d, want -1 — reopening a menu starts clean", got)
	}
}

// Arrowing past the page scrolls rather than moving an invisible cursor.
func TestMenuKeyNav_ScrollsToRevealSelection(t *testing.T) {
	sc := NewMenuScroll(DropdownScrollbarStyle, 20)
	sc.Configure(image.Rect(0, 0, 100, 60), 20, 3) // 20 rows, 3 visible
	host := &fakeNavHost{selectable: allSelectable(20), scroll: sc}
	nav := NewMenuKeyNav()
	press := navKeys(t, nav, host)

	for i := 0; i < 6; i++ {
		press(ebiten.KeyArrowDown) // cursor -> row 5
	}
	if got := nav.Selected(); got != 5 {
		t.Fatalf("cursor at %d, want 5", got)
	}
	vs := sc.ScrollBehavior().VS
	if 5 < vs.First || 5 >= vs.First+vs.Visible {
		t.Errorf("row 5 is outside the visible window [%d,%d) — arrowing past the page "+
			"must scroll the list to reveal the selection", vs.First, vs.First+vs.Visible)
	}
}

// Real Ebiten aliases KeyDown and KeyArrowDown, but the stub declares four distinct constants, and the code
// uses both spellings: the nav must answer to either.
func TestMenuKeyNav_AcceptsBothArrowKeySpellings(t *testing.T) {
	for _, tc := range []struct {
		name string
		down ebiten.Key
	}{
		{"KeyArrowDown", ebiten.KeyArrowDown},
		{"KeyDown", ebiten.KeyDown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := &fakeNavHost{selectable: allSelectable(3)}
			nav := NewMenuKeyNav()
			press := navKeys(t, nav, host)
			press(tc.down)
			if nav.Selected() != 0 {
				t.Errorf("%s did not move the highlight (Selected=%d). Real Ebiten "+
					"aliases the arrow spellings but the test stub does not, so the "+
					"nav must accept both.", tc.name, nav.Selected())
			}
		})
	}
}

// Folding the spellings must not double-fire on the real build, where they are one key.
func TestMenuKeyNav_AliasesShareOneEdgeSlot(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(5)}
	nav := NewMenuKeyNav()
	// What real Ebiten reports for one physical press.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyArrowDown || k == ebiten.KeyDown },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	nav.Update(host)
	if got := nav.Selected(); got != 0 {
		t.Errorf("one press reported on both alias constants advanced to %d, want 0 — "+
			"the aliases must share a single edge slot, not each fire once", got)
	}
}
