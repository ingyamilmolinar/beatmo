package ui

import (
	"image"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// wheelValueDrag presses just inside the barrel (not the center band or strip),
// drags the finger UP by dyUp px in a single move, then releases.
func wheelValueDrag(w *MobileWheelPopup, dyUp int) {
	sx := w.valRect.Min.X + 4
	sy := w.valRect.Min.Y + 4 // above the center band -> value drag, not editor
	w.HandleInput(sx, sy, true)
	w.HandleInput(sx, sy-dyUp, true)
	w.HandleInput(sx, sy-dyUp, false)
}

// A drag of exactly one notch advances the value by exactly ONE step (StepMul),
// not by a continuous pixel-scaled amount. Clicky, like a discrete knob.
func TestWheelDragClicky_OneStepPerNotch(t *testing.T) {
	k := newEndlessKnobForTest(0, 1000, 1.0) // StepMul=1, value 0.5 -> 500
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	start := realValue(k)
	wheelValueDrag(w, w.tickGap())
	if got := realValue(k); math.Abs(got-(start+1)) > 1e-9 {
		t.Fatalf("one-notch drag must advance exactly one StepMul: start=%v got=%v (want %v)", start, got, start+1)
	}
}

// A drag shorter than one notch must not move the value at all (sub-notch travel
// accumulates; it does not fly the value forward).
func TestWheelDragClicky_SubNotchNoChange(t *testing.T) {
	k := newEndlessKnobForTest(0, 1000, 1.0)
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	start := realValue(k)
	wheelValueDrag(w, w.tickGap()-1)
	if got := realValue(k); got != start {
		t.Fatalf("sub-notch drag must not change the value: start=%v got=%v", start, got)
	}
}

// A multi-notch drag advances exactly that many steps.
func TestWheelDragClicky_MultipleNotches(t *testing.T) {
	k := newEndlessKnobForTest(0, 1000, 1.0)
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	start := realValue(k)
	wheelValueDrag(w, w.tickGap()*3)
	if got := realValue(k); math.Abs(got-(start+3)) > 1e-9 {
		t.Fatalf("three-notch drag must advance exactly three steps: start=%v got=%v (want %v)", start, got, start+3)
	}
}

// The shared stepNotcher emits one step per pxPerNotch of travel, retains the
// remainder, and handles negatives — the cadence reused by every clicky control.
func TestStepNotcherCadence(t *testing.T) {
	n := newStepNotcher(10)
	if got := n.add(9); got != 0 {
		t.Fatalf("sub-notch (9 of 10) must yield 0 steps, got %d", got)
	}
	if got := n.add(1); got != 1 { // acc 9+1=10 -> 1 step, remainder 0
		t.Fatalf("crossing the notch must yield 1 step, got %d", got)
	}
	if got := n.add(25); got != 2 { // 25 -> 2 steps, remainder 5
		t.Fatalf("25px must yield 2 steps, got %d", got)
	}
	if got := n.add(-30); got != -2 { // 5-30=-25 -> -2 steps, remainder -5
		t.Fatalf("negative travel must yield -2 steps, got %d", got)
	}
}

// Enum wheels stay clicky too: one item per notch of drag.
func TestWheelDragClicky_EnumOneItemPerNotch(t *testing.T) {
	def := paramDefEnumForTest()
	k := &Knob{Discrete: true}
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Enum: def.Enum}
	k.Value = 0 // first item
	b := WheelBinding{Knob: k, Def: def, Discrete: true, OnChange: func() {}, OnCommit: func() {}, Title: func() string { return "Wave" }}
	w := NewMobileWheelPopup()
	w.Open(b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	// One notch DOWN advances exactly one enum item (list-scroll semantics).
	sx := w.valRect.Min.X + 4
	sy := w.valRect.Min.Y + 4
	w.HandleInput(sx, sy, true)
	w.HandleInput(sx, sy+w.tickGap(), true) // drag down one notch
	w.HandleInput(sx, sy+w.tickGap(), false)
	wantIdx := 1
	gotIdx := enumIndex(k.Value, len(def.Enum))
	if gotIdx != wantIdx {
		t.Fatalf("one-notch enum drag must advance exactly one item: got idx %d want %d", gotIdx, wantIdx)
	}
}

func paramDefEnumForTest() audio.ParamDef {
	return audio.ParamDef{Name: "wave", Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Tri"}}
}

// A single two-finger/trackpad scroll event carries a large magnitude; it must
// NOT dump many value steps at once. Scroll is paced, not magnitude-scaled.
func TestWheelScroll_LargeEventDoesNotFly(t *testing.T) {
	k := newEndlessKnobForTest(0, 1000, 1.0) // StepMul=1, start 500
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	cx := w.valRect.Min.X + 4
	cy := w.valRect.Min.Y + 4

	start := realValue(k)
	w.HandleWheel(cx, cy, 12) // one fast two-finger scroll event
	if moved := realValue(k) - start; moved > 1.0+1e-9 {
		t.Fatalf("a single scroll event must move at most one step, moved %v", moved)
	}
}

// Scroll is paced: it takes several wheel events to advance one value step.
func TestWheelScroll_PacedAcrossEvents(t *testing.T) {
	k := newEndlessKnobForTest(0, 1000, 1.0)
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	cx := w.valRect.Min.X + 4
	cy := w.valRect.Min.Y + 4

	start := realValue(k)
	w.HandleWheel(cx, cy, 1)
	if realValue(k) != start {
		t.Fatalf("first scroll event should not yet advance a step (paced); start=%v got=%v", start, realValue(k))
	}
	for i := 0; i < wheelStepEventsPerNotch; i++ {
		w.HandleWheel(cx, cy, 1)
	}
	if got := realValue(k); math.Abs(got-(start+1)) > 1e-9 {
		t.Fatalf("after a full notch of scroll events the value should advance exactly one step: start=%v got=%v", start, got)
	}
}
