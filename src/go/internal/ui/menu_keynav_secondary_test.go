//go:build test

// Shift+Enter fires the row's optional secondary action (the instrument menu's ★); a host without one gets
// plain Enter.

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// secondaryNavHost is a fakeNavHost that also records secondary activations.
type secondaryNavHost struct {
	fakeNavHost
	secondary []int
}

func (h *secondaryNavHost) MenuSecondary(i int) { h.secondary = append(h.secondary, i) }

// One frame with every key held, so a chord is the single gesture it is.
func navKeysMulti(t *testing.T, nav *MenuKeyNav, host MenuKeyNavHost) func(...ebiten.Key) {
	t.Helper()
	held := map[ebiten.Key]bool{}
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return held[k] },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	t.Cleanup(restore)
	return func(keys ...ebiten.Key) {
		for _, k := range keys {
			held[k] = true
		}
		nav.Update(host) // rising edge -> acts
		held = map[ebiten.Key]bool{}
		nav.Update(host) // release
	}
}

func TestMenuKeyNav_ShiftEnterFiresSecondaryNotActivate(t *testing.T) {
	host := &secondaryNavHost{fakeNavHost: fakeNavHost{selectable: allSelectable(3)}}
	nav := NewMenuKeyNav()
	press := navKeysMulti(t, nav, host)

	press(ebiten.KeyArrowDown) // cursor -> row 0
	press(ebiten.KeyShiftLeft, ebiten.KeyEnter)

	if len(host.secondary) != 1 || host.secondary[0] != 0 {
		t.Errorf("Shift+Enter fired secondary %v, want [0] — the ★ toggle must run "+
			"on the cursor row", host.secondary)
	}
	if len(host.activated) != 0 {
		t.Errorf("Shift+Enter also ACTIVATED rows %v; want none — starring an "+
			"instrument must not also select it", host.activated)
	}
}

// The stub declares only KeyShiftLeft/Right while real Ebiten aliases KeyShift, so both sides are checked:
// binding one spelling is how the arrows first shipped half-broken.
func TestMenuKeyNav_RightShiftEnterAlsoFiresSecondary(t *testing.T) {
	host := &secondaryNavHost{fakeNavHost: fakeNavHost{selectable: allSelectable(3)}}
	nav := NewMenuKeyNav()
	press := navKeysMulti(t, nav, host)

	press(ebiten.KeyArrowDown)
	press(ebiten.KeyShiftRight, ebiten.KeyEnter)

	if len(host.secondary) != 1 || host.secondary[0] != 0 {
		t.Errorf("right Shift+Enter fired secondary %v, want [0]", host.secondary)
	}
}

func TestMenuKeyNav_PlainEnterStillActivates(t *testing.T) {
	host := &secondaryNavHost{fakeNavHost: fakeNavHost{selectable: allSelectable(3)}}
	nav := NewMenuKeyNav()
	press := navKeysMulti(t, nav, host)

	press(ebiten.KeyArrowDown)
	press(ebiten.KeyEnter)

	if len(host.activated) != 1 || host.activated[0] != 0 {
		t.Errorf("plain Enter activated %v, want [0]", host.activated)
	}
	if len(host.secondary) != 0 {
		t.Errorf("plain Enter fired the SECONDARY action %v; want none", host.secondary)
	}
}

// Falls back to MenuActivate rather than swallowing the chord.
func TestMenuKeyNav_ShiftEnterActivatesWhenHostHasNoSecondary(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(3)}
	nav := NewMenuKeyNav()
	press := navKeysMulti(t, nav, host)

	press(ebiten.KeyArrowDown)
	press(ebiten.KeyShiftLeft, ebiten.KeyEnter)

	if len(host.activated) != 1 || host.activated[0] != 0 {
		t.Errorf("Shift+Enter on a host with no secondary action activated %v, want [0] "+
			"— it must fall back to plain Enter, not do nothing", host.activated)
	}
}

// Shift+Enter with no cursor must be inert: there is no row to star.
func TestMenuKeyNav_ShiftEnterWithNoCursorDoesNothing(t *testing.T) {
	host := &secondaryNavHost{fakeNavHost: fakeNavHost{selectable: allSelectable(3)}}
	nav := NewMenuKeyNav()
	press := navKeysMulti(t, nav, host)

	press(ebiten.KeyShiftLeft, ebiten.KeyEnter)

	if len(host.secondary) != 0 || len(host.activated) != 0 {
		t.Errorf("Shift+Enter with nothing highlighted fired secondary=%v activated=%v; "+
			"want neither", host.secondary, host.activated)
	}
}

// --- Reveal -----------------------------------------------------------------

// SetSelected is the hover path and does not scroll, so a programmatically placed cursor needs Reveal or it
// lands off-screen and invisible.
func TestMenuKeyNav_RevealScrollsAProgrammaticCursorIntoView(t *testing.T) {
	scroll := NewMenuScroll(DropdownScrollbarStyle, 20)
	host := &fakeNavHost{selectable: allSelectable(20), scroll: scroll}
	sb := host.MenuScrollBehavior()
	sb.VS.Total, sb.VS.Visible, sb.VS.First = 20, 5, 0

	nav := NewMenuKeyNav()
	nav.BindHost(host)
	nav.SetSelected(17)

	if sb.VS.First != 0 {
		t.Fatalf("harness: SetSelected already scrolled (First=%d)", sb.VS.First)
	}

	nav.Reveal(host)

	first, vis := sb.VS.First, sb.VS.Visible
	if 17 < first || 17 >= first+vis {
		t.Errorf("after Reveal the cursor 17 is still outside the visible page "+
			"[%d,%d)", first, first+vis)
	}
}

// Scrolling a visible cursor would yank the list under a user who only hovered.
func TestMenuKeyNav_RevealLeavesAVisibleCursorAlone(t *testing.T) {
	scroll := NewMenuScroll(DropdownScrollbarStyle, 20)
	host := &fakeNavHost{selectable: allSelectable(20), scroll: scroll}
	sb := host.MenuScrollBehavior()
	sb.VS.Total, sb.VS.Visible, sb.VS.First = 20, 5, 10

	nav := NewMenuKeyNav()
	nav.BindHost(host)
	nav.SetSelected(12)
	nav.Reveal(host)

	if sb.VS.First != 10 {
		t.Errorf("Reveal scrolled a cursor that was already visible: First 10 -> %d",
			sb.VS.First)
	}
}
