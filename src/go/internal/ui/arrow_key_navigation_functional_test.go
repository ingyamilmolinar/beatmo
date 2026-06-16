//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestFunctional_ArrowKeys_EQdBEditor reproduces the user flow: open an EQ dB
// value editor by clicking its readout, then press Left/Right arrows — the caret
// must move (and the grid must NOT pan). Drives the real Game.Update+Draw loop.
func TestFunctional_ArrowKeys_EQdBEditor(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 4; i++ { // settle + draw so dB readout rects exist
		fi.frame(t, g)
	}

	ez := g.drum.eqPanelZone
	if ez == nil {
		t.Skip("no eq panel zone")
	}
	idx := -1
	for i := 0; i < 10; i++ {
		if !ez.dbReadoutRect(i).Empty() {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Skip("no EQ dB readout rect after draw")
	}

	// Open the editor via a real click on the dB readout.
	r := ez.dbReadoutRect(idx)
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	if ez.paramEditor == nil || !ez.paramEditor.Active() {
		t.Fatalf("click did not open dB editor for band %d", idx)
	}

	startCursor := ez.paramEditor.ti.cursor
	if startCursor == 0 {
		t.Fatalf("precondition: prefilled value should put caret at end (>0), got %d", startCursor)
	}
	camBefore := g.cam.OffsetX

	// Press Left arrow through the real loop.
	fi.pressKey(t, g, ebiten.KeyLeft)

	if g.cam.OffsetX != camBefore {
		t.Errorf("Left arrow panned the grid while editing (OffsetX %v -> %v)", camBefore, g.cam.OffsetX)
	}
	if ez.paramEditor == nil || !ez.paramEditor.Active() {
		t.Fatalf("Left arrow unexpectedly closed the editor")
	}
	if ez.paramEditor.ti.cursor != startCursor-1 {
		t.Fatalf("Left arrow did NOT move caret: cursor=%d want %d", ez.paramEditor.ti.cursor, startCursor-1)
	}

	// Press Right arrow — caret back.
	fi.pressKey(t, g, ebiten.KeyRight)
	if ez.paramEditor.ti.cursor != startCursor {
		t.Fatalf("Right arrow did NOT move caret back: cursor=%d want %d", ez.paramEditor.ti.cursor, startCursor)
	}
}

// TestFunctional_ArrowKeys_SynthKnobEditor reproduces editing a synth knob value
// and navigating with arrows through the REAL Game.Update loop.
func TestFunctional_ArrowKeys_SynthKnobEditor(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()
	img := newTrackedImage("test.arrowsynth", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	g.drum.openSynthParamEditor(idx, "ut-chip-modular")
	if g.drum.paramEditor == nil || !g.drum.paramEditor.Active() {
		t.Fatalf("synth editor not active after open")
	}
	startCursor := g.drum.paramEditor.ti.cursor
	if startCursor == 0 {
		t.Fatalf("precondition: prefilled caret at end (>0), got %d", startCursor)
	}

	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	camBefore := g.cam.OffsetX

	fi.pressKey(t, g, ebiten.KeyLeft)

	if g.cam.OffsetX != camBefore {
		t.Errorf("Left arrow panned grid while editing synth value (%v -> %v)", camBefore, g.cam.OffsetX)
	}
	if g.drum.paramEditor == nil || !g.drum.paramEditor.Active() {
		t.Fatalf("Left arrow closed the synth editor")
	}
	if g.drum.paramEditor.ti.cursor != startCursor-1 {
		t.Fatalf("Left arrow did NOT move caret in real loop: cursor=%d want %d", g.drum.paramEditor.ti.cursor, startCursor-1)
	}
}

// TestFunctional_SoftKeyboardArrowMovesCaret reproduces the WASM bug: while a
// text field is focused the hidden soft-keyboard proxy holds keyboard focus, so
// the canvas/ebiten never sees the arrow keydown (isKeyPressed stays false). The
// proxy must forward Left/Right the way it forwards Esc. This test fires ONLY the
// forwarded signal (no canvas arrow key) and asserts the editor caret moves.
func TestFunctional_SoftKeyboardArrowMovesCaret(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 4; i++ {
		fi.frame(t, g)
	}
	ez := g.drum.eqPanelZone
	if ez == nil {
		t.Skip("no eq panel zone")
	}
	idx := -1
	for i := 0; i < 10; i++ {
		if !ez.dbReadoutRect(i).Empty() {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Skip("no EQ dB readout rect")
	}
	r := ez.dbReadoutRect(idx)
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	if ez.paramEditor == nil || !ez.paramEditor.Active() {
		t.Fatalf("click did not open dB editor")
	}
	startCursor := ez.paramEditor.ti.cursor
	if startCursor == 0 {
		t.Fatalf("precondition: caret at end (>0), got %d", startCursor)
	}

	// Crucially: NO canvas arrow key (fi.keys empty). Only the proxy's forwarded
	// signal, exactly as the WASM keydown handler will deliver it.
	SignalSoftKeyboardArrowLeft()
	fi.frame(t, g)
	if ez.paramEditor.ti.cursor != startCursor-1 {
		t.Fatalf("soft-keyboard Left did NOT move caret: cursor=%d want %d", ez.paramEditor.ti.cursor, startCursor-1)
	}

	SignalSoftKeyboardArrowRight()
	fi.frame(t, g)
	if ez.paramEditor.ti.cursor != startCursor {
		t.Fatalf("soft-keyboard Right did NOT move caret back: cursor=%d want %d", ez.paramEditor.ti.cursor, startCursor)
	}
}
