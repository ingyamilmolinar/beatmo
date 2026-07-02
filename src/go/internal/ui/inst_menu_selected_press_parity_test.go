//go:build test

package ui

import "testing"

// activeInstBtnPressed reports whether the instrument-menu row button bound to
// the menu's currently-selected instrument renders in the pressed-down ("keycap
// in") state, and whether any OTHER row is also pressed. The selected row must
// read as pressed (the visual "toggle pressed down" the user sees on desktop);
// exactly one row — the live selection — may be pressed at a time.
func activeInstBtnPressed(t *testing.T, comp *InstrumentMenuComponent) (selected, otherPressed bool) {
	t.Helper()
	ids := comp.VisibleInstIDs()
	btns := comp.InstBtns()
	active := comp.ActiveInstrumentID()
	for i, b := range btns {
		id := ""
		if i < len(ids) {
			id = ids[i]
		}
		if id == active {
			selected = b.Pressed()
		} else if b.Pressed() {
			otherPressed = true
		}
	}
	return selected, otherPressed
}

// TestInstSelectedButtonPressedParity is the cross-platform parity contract for
// the instrument picker's selected-row visual: on BOTH desktop and mobile, the
// button bound to the currently-selected instrument must render pressed-down
// (the "toggle pressed down" feedback), and no other row may be pressed. The
// mobile path used to fire selection via fireTapAt → OnClick, which bypassed the
// Button press lifecycle, so the selected mobile row stayed flat while the same
// desktop row read as pressed.
func TestInstSelectedButtonPressedParity(t *testing.T) {
	run := func(t *testing.T, mobile bool) {
		assertDefaultParityState(t)
		if mobile {
			forceSmallScreenForTest = true
			t.Cleanup(func() { forceSmallScreenForTest = false })
		}

		const W, H = 800, 600
		dv, cx, cy := newCategoryDrumView(t, W, H)
		comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

		// Select an instrument, then let the menu settle (it stays open for
		// audition on both platforms).
		if !pickVisibleInstrument(t, dv, comp, "snare", W, H) {
			t.Fatalf("snare not visible; ids=%v", comp.VisibleInstIDs())
		}
		idleFrames(dv, 2, W, H)()

		if got := comp.ActiveInstrumentID(); got != "snare" {
			t.Fatalf("active instrument = %q, want snare", got)
		}

		selected, otherPressed := activeInstBtnPressed(t, comp)
		if !selected {
			t.Errorf("mobile=%v: selected instrument row is NOT pressed-down "+
				"(missing the desktop toggle-pressed feedback)", mobile)
		}
		if otherPressed {
			t.Errorf("mobile=%v: a non-selected row is pressed — only the live "+
				"selection may read as pressed", mobile)
		}

		// Switching the selection moves the pressed state to the new row and
		// releases the previously-selected one.
		if !pickVisibleInstrument(t, dv, comp, "tom", W, H) {
			t.Fatalf("tom not visible; ids=%v", comp.VisibleInstIDs())
		}
		idleFrames(dv, 2, W, H)()

		if got := comp.ActiveInstrumentID(); got != "tom" {
			t.Fatalf("active instrument = %q, want tom", got)
		}
		selected, otherPressed = activeInstBtnPressed(t, comp)
		if !selected {
			t.Errorf("mobile=%v: re-selected row (tom) is NOT pressed-down", mobile)
		}
		if otherPressed {
			t.Errorf("mobile=%v: previously-selected row stayed pressed after "+
				"switching selection", mobile)
		}
	}

	t.Run("desktop", func(t *testing.T) { run(t, false) })
	t.Run("mobile", func(t *testing.T) { run(t, true) })
}
