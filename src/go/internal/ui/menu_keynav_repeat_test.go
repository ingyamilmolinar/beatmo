//go:build test

// Held arrows repeat on the menuRepeat curve. The load-bearing tests are the negative ones: a held Enter
// would preview, confirm, close and reopen, and a held Shift+Enter would flicker a favourite ★.

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Frames are 1-based from the first held frame; one nav.Update per frame, as a menu's hook does.
func holdKeys(t *testing.T, nav *MenuKeyNav, host MenuKeyNavHost, n int, observe func() int, keys ...ebiten.Key) []int {
	t.Helper()
	down := true
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool {
			if !down {
				return false
			}
			for _, want := range keys {
				if k == want {
					return true
				}
			}
			return false
		},
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	t.Cleanup(restore)
	_ = down

	var fired []int
	prev := observe()
	for f := 1; f <= n; f++ {
		nav.Update(host)
		if cur := observe(); cur != prev {
			fired = append(fired, f)
			prev = cur
		}
	}
	return fired
}

func selObserver(nav *MenuKeyNav) func() int { return func() int { return nav.Selected() } }

func TestMenuKeyNavRepeat_BriefHoldMovesExactlyOneRow(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(50)}
	nav := NewMenuKeyNav()
	// One frame short of the delay: still a single-step press.
	fired := holdKeys(t, nav, host, menuRepeat.Delay, selObserver(nav), ebiten.KeyArrowDown)
	if len(fired) != 1 {
		t.Errorf("a %d-frame hold moved %d times (frames %v), want exactly 1 — the "+
			"repeat must not start inside the %d-frame delay",
			menuRepeat.Delay, len(fired), fired, menuRepeat.Delay)
	}
}

func TestMenuKeyNavRepeat_FirstRepeatLandsAfterTheDelay(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(50)}
	nav := NewMenuKeyNav()
	fired := holdKeys(t, nav, host, menuRepeat.Delay+menuRepeat.Interval+2,
		selObserver(nav), ebiten.KeyArrowDown)
	if len(fired) < 2 {
		t.Fatalf("held arrow fired %d times (%v), want at least 2 — repeat never started",
			len(fired), fired)
	}
	if got, want := fired[1], menuRepeat.Delay+menuRepeat.Interval; got != want {
		t.Errorf("first repeat on frame %d, want %d (delay %d + interval %d)",
			got, want, menuRepeat.Delay, menuRepeat.Interval)
	}
}

func TestMenuKeyNavRepeat_Accelerates(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(400)}
	nav := NewMenuKeyNav()
	fired := holdKeys(t, nav, host, 300, selObserver(nav), ebiten.KeyArrowDown)
	if len(fired) < 12 {
		t.Fatalf("only %d moves in 300 frames (%v)", len(fired), fired)
	}
	firstGap := fired[2] - fired[1]
	lastGap := fired[len(fired)-1] - fired[len(fired)-2]
	if lastGap >= firstGap {
		t.Errorf("no acceleration: early gap %d frames, late gap %d frames", firstGap, lastGap)
	}
	if lastGap < menuRepeat.MinInterval {
		t.Errorf("late gap %d is faster than MinInterval %d", lastGap, menuRepeat.MinInterval)
	}
}

func TestMenuKeyNavRepeat_ReleaseResetsTheHold(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(50)}
	nav := NewMenuKeyNav()
	// Hold long enough to be mid-repeat.
	holdKeys(t, nav, host, menuRepeat.Delay+menuRepeat.Interval+5, selObserver(nav), ebiten.KeyArrowDown)
	// Release for a frame, then a fresh brief hold must be a single step again.
	holdKeys(t, nav, host, 3, selObserver(nav)) // no keys = released
	fired := holdKeys(t, nav, host, menuRepeat.Delay, selObserver(nav), ebiten.KeyArrowDown)
	if len(fired) != 1 {
		t.Errorf("after releasing, a fresh %d-frame hold moved %d times (%v), want 1 "+
			"— the held counter did not reset", menuRepeat.Delay, len(fired), fired)
	}
}

func TestMenuKeyNavRepeat_UpRepeatsToo(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(50)}
	nav := NewMenuKeyNav()
	fired := holdKeys(t, nav, host, menuRepeat.Delay+menuRepeat.Interval+2,
		selObserver(nav), ebiten.KeyArrowUp)
	if len(fired) < 2 {
		t.Errorf("held ArrowUp fired %d times (%v), want repeats", len(fired), fired)
	}
}

// --- The negatives: keys that must NOT repeat ------------------------------

func TestMenuKeyNavRepeat_EnterNeverRepeats(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(50)}
	nav := NewMenuKeyNav()
	nav.BindHost(host)
	nav.SetSelected(3)

	obs := func() int { return len(host.activated) }
	holdKeys(t, nav, host, 400, obs, ebiten.KeyEnter)

	if len(host.activated) != 1 {
		t.Errorf("Enter held for 400 frames activated %d times (%v), want exactly 1 "+
			"— a repeating Enter would preview, confirm and close in one gesture",
			len(host.activated), host.activated)
	}
}

func TestMenuKeyNavRepeat_ShiftEnterNeverRepeats(t *testing.T) {
	host := &secondaryNavHost{fakeNavHost: fakeNavHost{selectable: allSelectable(50)}}
	nav := NewMenuKeyNav()
	nav.BindHost(host)
	nav.SetSelected(3)

	obs := func() int { return len(host.secondary) }
	holdKeys(t, nav, host, 400, obs, ebiten.KeyShiftLeft, ebiten.KeyEnter)

	if len(host.secondary) != 1 {
		t.Errorf("Shift+Enter held for 400 frames starred %d times, want exactly 1 — "+
			"a repeating ★ toggle flickers the favourite on and off", len(host.secondary))
	}
}

func TestMenuKeyNavRepeat_HomeAndEndNeverRepeat(t *testing.T) {
	for _, k := range []ebiten.Key{ebiten.KeyHome, ebiten.KeyEnd} {
		host := &fakeNavHost{selectable: allSelectable(50)}
		nav := NewMenuKeyNav()
		fired := holdKeys(t, nav, host, 400, selObserver(nav), k)
		if len(fired) != 1 {
			t.Errorf("%v held for 400 frames moved %d times (%v), want 1 — an absolute "+
				"jump has nowhere to repeat to", k, len(fired), fired)
		}
	}
}

// --- Paging repeats --------------------------------------------------------

func TestMenuKeyNavRepeat_PageDownRepeats(t *testing.T) {
	scroll := NewMenuScroll(DropdownScrollbarStyle, 20)
	host := &fakeNavHost{selectable: allSelectable(400), scroll: scroll}
	sb := host.MenuScrollBehavior()
	sb.VS.Total, sb.VS.Visible, sb.VS.First = 400, 10, 0

	nav := NewMenuKeyNav()
	fired := holdKeys(t, nav, host, menuRepeat.Delay+menuRepeat.Interval+2,
		selObserver(nav), ebiten.KeyPageDown)
	if len(fired) < 2 {
		t.Errorf("held PageDown fired %d times (%v), want repeats", len(fired), fired)
	}
}

// --- Clamping --------------------------------------------------------------

// Clamps, never wraps, and never spins past the end.
func TestMenuKeyNavRepeat_HeldArrowClampsAtTheLastRow(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(6)}
	nav := NewMenuKeyNav()
	holdKeys(t, nav, host, 400, selObserver(nav), ebiten.KeyArrowDown)
	if got := nav.Selected(); got != 5 {
		t.Errorf("after holding Down on a 6-row list the cursor is at %d, want the "+
			"last row 5 — the repeat wrapped or ran past the end", got)
	}
}

// A menu reopened with an arrow still down must wait for a fresh edge and the initial delay, not resume
// mid-hold.
func TestMenuKeyNavRepeat_ResetForgetsHeldKeys(t *testing.T) {
	host := &fakeNavHost{selectable: allSelectable(50)}
	nav := NewMenuKeyNav()

	// Hold well past the delay so the slot is mid-repeat.
	holdKeys(t, nav, host, menuRepeat.Delay+menuRepeat.Interval*3, selObserver(nav), ebiten.KeyArrowDown)

	// The menu closes and reopens with the key still down.
	nav.Reset()

	fired := holdKeys(t, nav, host, menuRepeat.Delay, selObserver(nav), ebiten.KeyArrowDown)
	if len(fired) != 1 {
		t.Errorf("after Reset, a %d-frame hold moved %d times (%v), want 1 — the "+
			"reopened menu resumed a stale hold and skipped its initial delay",
			menuRepeat.Delay, len(fired), fired)
	}
	if len(fired) == 1 && fired[0] != 1 {
		t.Errorf("after Reset the first move landed on frame %d, want the press edge "+
			"(frame 1)", fired[0])
	}
}

// --- Grid menus ------------------------------------------------------------

// The colour picker / note matrix shape: Up/Down move a row, Left/Right one cell.
type gridRepeatHost struct {
	fakeNavHost
	cols int
}

func (h *gridRepeatHost) MenuGridCols() int { return h.cols }

// Pickers ride the same MenuKeyNav with Left/Right as their main axis, so the repeat must reach those keys too.
func TestMenuKeyNavRepeat_GridHorizontalRepeats(t *testing.T) {
	host := &gridRepeatHost{fakeNavHost: fakeNavHost{selectable: allSelectable(96)}, cols: 12}
	nav := NewMenuKeyNav()
	fired := holdKeys(t, nav, host, menuRepeat.Delay+menuRepeat.Interval+2,
		selObserver(nav), ebiten.KeyArrowRight)
	if len(fired) < 2 {
		t.Errorf("held ArrowRight on a grid fired %d times (%v), want repeats",
			len(fired), fired)
	}
}

func TestMenuKeyNavRepeat_GridVerticalMovesAWholeRowPerRepeat(t *testing.T) {
	const cols = 12
	host := &gridRepeatHost{fakeNavHost: fakeNavHost{selectable: allSelectable(96)}, cols: cols}
	nav := NewMenuKeyNav()
	nav.BindHost(host)

	// The press edge enters at cell 0, then exactly one repeat falls inside the window.
	fired := holdKeys(t, nav, host, menuRepeat.Delay+menuRepeat.Interval+2,
		selObserver(nav), ebiten.KeyArrowDown)
	if len(fired) != 2 {
		t.Fatalf("expected the press edge plus exactly one repeat, got %d moves (%v)",
			len(fired), fired)
	}
	if got := nav.Selected(); got != cols {
		t.Errorf("after the press edge plus one repeat the grid cursor is at cell %d, "+
			"want %d — a vertical repeat must move a whole ROW, not one cell", got, cols)
	}
}
