//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A mobile layout driven by mouse and keyboard (a narrow desktop window) still
// takes BPM input from the keyboard: there is no touch to open a native <input>.
func TestFunctional_MobileLayout_DesktopKeyboard_BPM(t *testing.T) {
	g := newDesktopGame(t)
	// Force the mobile LAYOUT (as a narrow desktop-browser viewport would). Under
	// the test build SetForceMobileProfile is a no-op, so set the flag directly.
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

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

	// 1. Click the BPM readout: it opens the shared value wheel (typed numeric
	// entry was replaced by the wheel on both platforms).
	r := box.Rect
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	w := g.drum.valueWheelPopup
	if w == nil || !w.IsOpen() {
		t.Fatalf("clicking the BPM readout did not open the value wheel")
	}

	// 2. Physical arrow keys step the tempo (proves key presses register on a
	// mobile layout driven by a mouse + keyboard).
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
