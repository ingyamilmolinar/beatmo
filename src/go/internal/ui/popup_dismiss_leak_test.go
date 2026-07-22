//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestTapOutsidePopupDoesNotOpenElementBeneath reproduces the reported leak: with
// wheel popup A open, a mobile tap on a DIFFERENT card B (outside popup A) must
// only CLOSE A — it must not also open B. On the real touch pipeline the
// touch-DOWN closes A (overlay click-outside) and the lift's injectTouchTap
// replays the SAME gesture; with A now closed, that replay used to dispatch to
// card B and open it ("I can still open other elements").
func TestTapOutsidePopupDoesNotOpenElementBeneath(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum

	aIdx := synthIdxByParam(t, g, "filter_cutoff")
	_, aRect := firstContinuousSynthControl(t, g)
	var bRect image.Rectangle
	bIdx := -1
	for i, k := range dv.instEditorControls {
		if i == aIdx {
			continue
		}
		kr := k.Rect()
		if kr.Empty() || kr.Overlaps(aRect) {
			continue
		}
		bRect, bIdx = kr, i
		break
	}
	if bIdx < 0 {
		t.Skip("no second non-overlapping card found at this layout")
	}
	ax, ay := aRect.Min.X+4, aRect.Max.Y-4
	bx, by := bRect.Min.X+4, bRect.Max.Y-4

	globalTouchState.Reset()
	t.Cleanup(globalTouchState.Reset)
	t.Cleanup(resetTouchOverride)
	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	realTap := func(id ebiten.TouchID, x, y int) {
		mock.addTouch(id, x, y)
		g.Update()
		g.Update()
		mock.removeTouch(id)
		g.Update()
		g.Update()
		g.Update()
	}

	// Open popup A.
	realTap(1, ax, ay)
	if !dv.synthWheelPopup.IsOpen() || dv.synthWheelPopup.Anchor() != aRect {
		t.Fatalf("precondition: tapping card A must open popup A (open=%v anchor=%v want %v)",
			dv.synthWheelPopup.IsOpen(), dv.synthWheelPopup.Anchor(), aRect)
	}

	// Tap card B (outside popup A). Must close A and NOT open B.
	realTap(2, bx, by)
	if dv.synthWheelPopup.IsOpen() && dv.synthWheelPopup.Anchor() == bRect {
		t.Fatalf("LEAK: tapping outside popup A (on card B) closed A but ALSO opened B's popup "+
			"(anchor=%v == B rect). The injected replay of the dismiss tap reached the element beneath.", bRect)
	}
}

// TestSynthCardTapViaRealPipelineStaysOpen is the faithful full-pipeline version
// of the open-then-close bug: a mobile tap on a synth card opens its popup on the
// touch-DOWN; the lift must NOT replay onto the same card and close it. Drives the
// REAL globalTouchState → Game.Update → injectTouchTap decision (unlike the
// SetInputForTest variant, this exercises the injection GATE).
func TestSynthCardTapViaRealPipelineStaysOpen(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	_, r := firstContinuousSynthControl(t, g)
	cx, cy := r.Min.X+4, r.Max.Y-4

	globalTouchState.Reset()
	t.Cleanup(globalTouchState.Reset)
	t.Cleanup(resetTouchOverride)
	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// Capture-on-press: touch-DOWN captures the gesture (so a sweep could scroll)
	// but must NOT open the popup yet.
	mock.addTouch(1, cx, cy)
	g.Update()
	g.Update()
	if dv.synthWheelPopup.IsOpen() {
		t.Fatal("popup opened on touch-down; it must wait for the lift so a swipe can scroll")
	}
	// Lift without moving = a tap: the popup opens on release and must STAY open
	// through the injected-replay window (the self-close bug this test guards).
	mock.removeTouch(1)
	g.Update()
	g.Update()
	g.Update()

	if !dv.synthWheelPopup.IsOpen() || dv.synthWheelPopup.Anchor() != r {
		t.Fatalf("tap did not open the popup on release, or the injected replay closed/rebound it "+
			"(open=%v anchor=%v want %v)", dv.synthWheelPopup.IsOpen(), dv.synthWheelPopup.Anchor(), r)
	}
}
