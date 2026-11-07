//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A mobile layout driven by a mouse (a narrow window, a touch laptop) must still take keys: input routes by
// the opening gesture's pointer type, never by device capability.
func TestFunctional_MobileLayout_DesktopKeyboard_BPM(t *testing.T) {
	g := newDesktopGame(t)
	// SetForceMobileProfile is a no-op under the test build.
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

	// Opens the shared value wheel.
	r := box.Rect
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	w := g.drum.valueWheelPopup
	if w == nil || !w.IsOpen() {
		t.Fatalf("clicking the BPM readout did not open the value wheel")
	}

	// Physical arrows step the tempo.
	for i := 0; i < 3; i++ {
		fi.pressKey(t, g, ebiten.KeyArrowUp)
	}
	if got := g.drum.BPM(); got != orig+3 {
		t.Fatalf("ArrowUp presses did not register on mobile layout: BPM %d, want %d "+
			"(physical key presses are being dropped)", got, orig+3)
	}

	// 3. Enter commits and closes.
	fi.pressKey(t, g, ebiten.KeyEnter)
	if w.IsOpen() {
		t.Fatalf("Enter did not close the BPM wheel")
	}
	if g.drum.BPM() != orig+3 {
		t.Fatalf("committed BPM = %d, want %d", g.drum.BPM(), orig+3)
	}
}
