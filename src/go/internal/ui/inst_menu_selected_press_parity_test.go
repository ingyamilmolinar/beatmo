//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Exactly one row, the live selection, may read as pressed.
func activeInstBtnPressed(t *testing.T, comp *InstrumentMenuComponent) (selected, otherPressed bool) {
	t.Helper()
	ids := comp.VisibleInstIDs()
	btns := comp.InstBtns()
	active := comp.ActiveInstrumentID()
	for i, b := range btns {
		id := ""
		if i < len(ids) {
			id = string(ids[i])
		}
		if instrument.ID(id) == active {
			selected = b.Pressed()
		} else if b.Pressed() {
			otherPressed = true
		}
	}
	return selected, otherPressed
}

// On both platforms the selected row renders pressed-down and no other: a mobile pick that skips the Button
// press lifecycle leaves it flat while desktop reads pressed.
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

		if !pickVisibleInstrument(t, dv, comp, roster.Snare, W, H) {
			t.Fatalf("snare not visible; ids=%v", comp.VisibleInstIDs())
		}
		idleFrames(dv, 2, W, H)()

		if got := comp.ActiveInstrumentID(); got != roster.Snare {
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

		if !pickVisibleInstrument(t, dv, comp, roster.Tom, W, H) {
			t.Fatalf("tom not visible; ids=%v", comp.VisibleInstIDs())
		}
		idleFrames(dv, 2, W, H)()

		if got := comp.ActiveInstrumentID(); got != roster.Tom {
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
