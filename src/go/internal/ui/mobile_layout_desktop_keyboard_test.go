//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestFunctional_MobileLayout_DesktopKeyboard_BPM reproduces the reported bug:
// on a browser rendering the MOBILE LAYOUT and driven with a MOUSE (a narrow
// desktop window, OR — the case that shipped broken — a touch-capable laptop /
// DevTools device-mode used with a mouse), clicking the BPM readout opens the
// editor and shows a blinking cursor, but no key press registers.
//
// Root cause: ParamValueEditor.Update took the mobile native-<input> branch on
// Profile().IsMobile() (+ device touch capability) and returned before running
// keyboard handling. The native <input> is created only inside a `touchend`
// handler, which never fires on a MOUSE gesture — so the editor is inert.
//
// The fix routes input by the OPENING GESTURE's pointer type (lastPointerWasTouch),
// not device capability: a mouse-opened editor in mobile layout uses the keyboard
// path. Here lastPointerWasTouchForTest=false models the mouse gesture.
func TestFunctional_MobileLayout_DesktopKeyboard_BPM(t *testing.T) {
	g := newDesktopGame(t)
	// Force the mobile LAYOUT (as a narrow desktop-browser viewport would). Under
	// the test build SetForceMobileProfile is a no-op, so set the flag directly.
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()
	// ...and model a MOUSE opening gesture (no native <input> will be created).
	prevTouch := lastPointerWasTouchForTest
	lastPointerWasTouchForTest = false
	defer func() { lastPointerWasTouchForTest = prevTouch }()

	if !Profile().IsMobile() {
		t.Fatalf("precondition: expected mobile layout to be forced")
	}

	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()

	// Settle so transport layout assigns the BPM box rect.
	fi.frame(t, g)
	fi.frame(t, g)

	box := g.drum.bpmBox()
	if box == nil || box.Rect.Empty() {
		t.Fatalf("BPM box has no rect (rect=%v) — cannot click it", box.Rect)
	}
	orig := g.drum.BPM()

	// 1. Click the BPM readout to open the shared editor.
	r := box.Rect
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	ed := g.drum.transportZone.paramEditor
	if ed == nil || !ed.Active() {
		t.Fatalf("clicking the BPM readout did not open the editor")
	}

	// 2. Clear the prefilled tempo with Backspace, proving backspace registers.
	for i := 0; i < 6 && ed.ti.Value() != ""; i++ {
		fi.pressKey(t, g, ebiten.KeyBackspace)
	}
	if got := ed.ti.Value(); got != "" {
		t.Fatalf("Backspace did not clear the BPM editor on mobile layout: text = %q "+
			"(physical key presses are being dropped)", got)
	}

	// 3. Type a new tempo with a physical keyboard (Ebiten inputChars).
	fi.chars = []rune{'9', '0'}
	fi.frame(t, g)
	if got := ed.ti.Value(); got != "90" {
		t.Fatalf("physical key presses did not register in the BPM editor on mobile "+
			"layout: editor text = %q, want %q", got, "90")
	}

	// 4. Enter commits the value through to the engine.
	fi.pressKey(t, g, ebiten.KeyEnter)
	if ed.Active() {
		t.Fatalf("Enter did not close the BPM editor")
	}
	if g.drum.BPM() == orig {
		t.Fatalf("Enter did not commit the edit: BPM still %d (orig)", g.drum.BPM())
	}
	if g.drum.BPM() != 90 {
		t.Fatalf("committed BPM = %d, want 90", g.drum.BPM())
	}
}
