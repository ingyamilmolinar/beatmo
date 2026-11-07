//go:build test

// Choosing a DIFFERENT instrument applies it and keeps the menu open for auditioning; choosing the applied one
// confirms and closes. The rule is positional, not a double-click timer, and pointer and keyboard both go
// through applyInstrument. Driven through DrumView.Update so the keys are really pumped.

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Rising-edge detection needs one frame held and one released.
func pressKeyDrumView(dv *DrumView, k ebiten.Key, w, h int) {
	held := k
	r := SetInputForTest(
		func() (int, int) { return -100, -100 }, // pointer parked off every menu
		func(ebiten.MouseButton) bool { return false },
		func(key ebiten.Key) bool { return key == held },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	dv.Update() // rising edge -> acts
	held = -1
	dv.Update() // release
	r()
}

// pressChordDrumView is pressKeyDrumView for a chord (Shift+Enter).
func pressChordDrumView(dv *DrumView, w, h int, keys ...ebiten.Key) {
	down := true
	r := SetInputForTest(
		func() (int, int) { return -100, -100 },
		func(ebiten.MouseButton) bool { return false },
		func(key ebiten.Key) bool {
			if !down {
				return false
			}
			for _, k := range keys {
				if k == key {
					return true
				}
			}
			return false
		},
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	dv.Update()
	down = false
	dv.Update()
	r()
}

func navToInstrument(t *testing.T, dv *DrumView, comp *InstrumentMenuComponent, want instrument.ID, w, h int) bool {
	t.Helper()
	nav := comp.MenuKeyNavForTest()
	ids := comp.FilteredInstIDsForTest()
	target := -1
	for i, id := range ids {
		if id == want {
			target = i
			break
		}
	}
	if target < 0 {
		return false
	}
	// Home first so the walk is deterministic regardless of the starting cursor.
	pressKeyDrumView(dv, ebiten.KeyHome, w, h)
	for guard := 0; nav.Selected() != target && guard < len(ids)+2; guard++ {
		pressKeyDrumView(dv, ebiten.KeyArrowDown, w, h)
	}
	if nav.Selected() != target {
		t.Fatalf("could not drive the cursor to %q (index %d); stuck at %d", want, target, nav.Selected())
	}
	return true
}

// Enter does what a click does: apply and stay open, so the user can audition.
func TestInstMenuEnterPreviewsWithoutClosing(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)
	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	if !navToInstrument(t, dv, comp, roster.Snare, W, H) {
		t.Fatalf("snare not in the filtered list: %v", comp.FilteredInstIDsForTest())
	}
	pressKeyDrumView(dv, ebiten.KeyEnter, W, H)
	idleFrames(dv, 2, W, H)()

	if got := dv.Rows[0].Instrument; got != roster.Snare {
		t.Errorf("Enter applied %q, want \"snare\" — Enter must preview the highlighted row", got)
	}
	if !dv.IsInstMenuOpen() {
		t.Error("Enter closed the menu on the FIRST press; it must stay open so the " +
			"user can audition more instruments (same rule as a click)")
	}
}

// TestInstMenuSecondEnterConfirmsAndCloses is the "double Enter" half.
func TestInstMenuSecondEnterConfirmsAndCloses(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)
	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	navToInstrument(t, dv, comp, roster.Snare, W, H)
	pressKeyDrumView(dv, ebiten.KeyEnter, W, H) // preview
	idleFrames(dv, 2, W, H)()
	pressKeyDrumView(dv, ebiten.KeyEnter, W, H) // confirm
	idleFrames(dv, 2, W, H)()

	if dv.IsInstMenuOpen() {
		t.Error("the second Enter on the already-previewed row did not close the menu; " +
			"re-selecting is the confirm gesture")
	}
	if got := dv.Rows[0].Instrument; got != roster.Snare {
		t.Errorf("confirming changed the instrument to %q, want it to stay \"snare\"", got)
	}
}

// Moving the cursor first makes the next Enter a fresh preview, not a confirm of the previous row.
func TestInstMenuEnterOnDifferentRowPreviewsAgain(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)
	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	navToInstrument(t, dv, comp, roster.Snare, W, H)
	pressKeyDrumView(dv, ebiten.KeyEnter, W, H)
	idleFrames(dv, 2, W, H)()

	navToInstrument(t, dv, comp, roster.Tom, W, H)
	pressKeyDrumView(dv, ebiten.KeyEnter, W, H)
	idleFrames(dv, 2, W, H)()

	if got := dv.Rows[0].Instrument; got != roster.Tom {
		t.Errorf("Enter on a DIFFERENT row applied %q, want \"tom\"", got)
	}
	if !dv.IsInstMenuOpen() {
		t.Error("Enter on a different row closed the menu; only re-selecting the row " +
			"already applied is the confirm")
	}
}

func TestInstMenuClickOnAlreadySelectedRowCloses(t *testing.T) {
	assertDefaultParityState(t)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)
	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	if !pickVisibleInstrument(t, dv, comp, roster.Snare, W, H) {
		t.Fatalf("snare not visible; ids=%v", comp.VisibleInstIDs())
	}
	idleFrames(dv, 2, W, H)()
	if !dv.IsInstMenuOpen() {
		t.Fatal("harness: the first click must leave the menu open (preview)")
	}

	if !pickVisibleInstrument(t, dv, comp, roster.Snare, W, H) {
		t.Fatalf("snare no longer visible; ids=%v", comp.VisibleInstIDs())
	}
	idleFrames(dv, 2, W, H)()

	if dv.IsInstMenuOpen() {
		t.Error("clicking the already-previewed row left the menu open; re-selecting " +
			"confirms, whichever input did it")
	}
}

// After a click the keyboard cursor must sit on what was clicked, or Enter re-applies the row the cursor was left
// on (the colour picker's syncCursorToApplied exists for the same reason).
func TestInstMenuClickThenEnterConfirms(t *testing.T) {
	assertDefaultParityState(t)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)
	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	if !pickVisibleInstrument(t, dv, comp, roster.Tom, W, H) {
		t.Fatalf("tom not visible; ids=%v", comp.VisibleInstIDs())
	}
	idleFrames(dv, 2, W, H)()

	ids := comp.FilteredInstIDsForTest()
	sel := comp.MenuKeyNavForTest().Selected()
	if sel < 0 || sel >= len(ids) || ids[sel] != roster.Tom {
		t.Fatalf("after clicking \"tom\" the keyboard cursor is at %d (%v); it must "+
			"follow the pointer so Enter confirms what was just clicked", sel, ids)
	}

	pressKeyDrumView(dv, ebiten.KeyEnter, W, H)
	idleFrames(dv, 2, W, H)()

	if dv.IsInstMenuOpen() {
		t.Error("Enter straight after clicking a row did not confirm/close")
	}
	if got := dv.Rows[0].Instrument; got != roster.Tom {
		t.Errorf("confirm changed the instrument to %q, want \"tom\"", got)
	}
}
