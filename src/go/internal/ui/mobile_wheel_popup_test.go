// src/go/internal/ui/mobile_wheel_popup_test.go
package ui

import (
	"image"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func wheelTestBinding(k *Knob) (*WheelBinding, *int) {
	commits := 0
	b := &WheelBinding{
		Knob:     k,
		Def:      audio.ParamDef{Name: "test", Min: k.Scale.Min, Max: k.Scale.Max},
		OnChange: func() {},
		OnCommit: func() { commits++ },
		Title:    func() string { return "Test" },
	}
	return b, &commits
}

// TestWheelDownGesturesAllIncrease locks the cross-input barrel invariant: a
// DOWNWARD gesture raises the value ("the wheel turns down, higher numbers descend
// to the center selector"). Finger drag DOWN and mouse/two-finger scroll DOWN must
// BOTH increase. They read different sign sources — pointer-y (my increases
// downward) vs ebiten.Wheel's wy (POSITIVE for a downward scroll on the target
// device) — so this guards against re-inverting one path without the other (the
// bug that recurred while tuning the wheel direction).
func TestWheelDownGesturesAllIncrease(t *testing.T) {
	// Finger drag DOWN (pointer y increases) increases.
	k1 := newEndlessKnobForTest(0, 200, 1.0)
	b1, _ := wheelTestBinding(k1)
	w1 := NewMobileWheelPopup()
	w1.Open(*b1, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	cx1 := w1.Rect().Min.X + 20
	y0 := w1.Rect().Min.Y + w1.Rect().Dy()/2
	w1.HandleInput(cx1, y0, true)
	w1.HandleInput(cx1, y0+knobEndlessPxPerNotch*10, true) // drag DOWN
	w1.HandleInput(cx1, y0+knobEndlessPxPerNotch*10, false)
	if realValue(k1) <= 100 {
		t.Fatalf("finger drag DOWN must increase, got %v", realValue(k1))
	}

	// Scroll DOWN (positive ebiten wy on the target => positive steps) increases —
	// same direction as the finger drag.
	k2 := newEndlessKnobForTest(0, 200, 1.0)
	b2, _ := wheelTestBinding(k2)
	w2 := NewMobileWheelPopup()
	w2.Open(*b2, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	cx2 := w2.valRect.Min.X + 4
	cy2 := (w2.valRect.Min.Y + w2.valRect.Max.Y) / 2
	for i := 0; i < wheelStepEventsPerNotch+1; i++ {
		w2.HandleWheel(cx2, cy2, 3) // scroll DOWN
	}
	if realValue(k2) <= 100 {
		t.Fatalf("scroll DOWN (positive wy) must increase, got %v", realValue(k2))
	}
}

func TestWheelDragDownIncreasesValue(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0) // value 0.5 -> real 100
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	cx := w.Rect().Min.X + 20
	// Barrel: higher values sit at the top. Dragging the finger DOWN (y increases)
	// pulls those higher rows down toward the center selector => value increases.
	y0 := w.Rect().Min.Y + w.Rect().Dy()/2
	w.HandleInput(cx, y0, true)
	w.HandleInput(cx, y0+knobEndlessPxPerNotch*10, true)
	w.HandleInput(cx, y0+knobEndlessPxPerNotch*10, false)

	if got := realValue(k); got <= 100 {
		t.Fatalf("drag down should increase value past 100, got %v", got)
	}
}

func TestWheelDragUpDecreasesValue(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	cx := w.Rect().Min.X + 20
	y0 := w.Rect().Min.Y + w.Rect().Dy()/2
	w.HandleInput(cx, y0, true)
	w.HandleInput(cx, y0-knobEndlessPxPerNotch*10, true)
	w.HandleInput(cx, y0-knobEndlessPxPerNotch*10, false)
	if got := realValue(k); got >= 100 {
		t.Fatalf("drag up should decrease value below 100, got %v", got)
	}
}

// Transactional model: a drag+release previews the change but DEFERS the undo
// commit; the single commit fires on Accept (Enter / tap-away), not on release.
func TestWheelReleaseDefersCommitToAccept(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	b, commits := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	cx := w.Rect().Min.X + 20
	y0 := w.Rect().Min.Y + w.Rect().Dy()/2
	gap := Profile().DensityValues().MobileWheelTickGap
	w.HandleInput(cx, y0, true)
	w.HandleInput(cx, y0-gap*20, true) // drag far enough to cross value notches
	w.HandleInput(cx, y0-gap*20, false)
	if *commits != 0 {
		t.Fatalf("release must not commit (deferred to Accept), got %d", *commits)
	}
	w.Accept()
	if *commits != 1 {
		t.Fatalf("expected exactly 1 commit on Accept, got %d", *commits)
	}
}

func TestWheelClampsAtMax(t *testing.T) {
	k := newEndlessKnobForTest(0, 10, 1.0)
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	cx := w.Rect().Min.X + 20
	y0 := w.Rect().Min.Y + w.Rect().Dy()/2
	w.HandleInput(cx, y0, true)
	w.HandleInput(cx, y0+knobEndlessPxPerNotch*100000, true) // drag down = increase toward max
	w.HandleInput(cx, y0+knobEndlessPxPerNotch*100000, false)
	if got := realValue(k); math.Abs(got-10) > 1e-6 {
		t.Fatalf("clamp at max: got %v want 10", got)
	}
}

// TestWheelResolutionStripCapturesUnderHandler drives the resolution-strip drag
// through the PORTAL HANDLER (mobileWheelHitHandler) rather than HandleInput
// directly — this is the path that the tree dispatcher uses. The bug was that
// OnPress returned InputConsumed (not InputCaptured) for a res-strip press, so
// the tree never set capturedHandler and OnDrag was never delivered.
func TestWheelResolutionStripCapturesUnderHandler(t *testing.T) {
	def := audio.ParamDef{Name: "cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	k := &Knob{Endless: true}
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Unit: def.Unit}
	k.Value = 0.5
	badge := NewKnobStepBadge(def)
	k.StepMul = badge.Step()
	startStep := k.StepMul

	b := WheelBinding{
		Knob: k, Badge: badge, Def: def,
		OnChange: func() {}, OnResolution: func() {},
		Title: func() string { return "Cutoff" },
	}
	w := NewMobileWheelPopup()
	w.Open(b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	if w.resRect.Empty() {
		t.Skip("no resolution strip (discrete param or zero width)")
	}

	h := &mobileWheelHitHandler{popup: w}
	rx := w.resRect.Min.X + 2
	y0 := w.resRect.Min.Y + w.resRect.Dy()/2

	// Press on the resolution strip — must return InputCaptured so the tree
	// routes subsequent OnDrag calls to this handler.
	res := h.OnPress(rx, y0)
	if res != InputCaptured {
		t.Fatalf("res-strip OnPress must return InputCaptured (got %v); without capture the tree never delivers OnDrag and the rung never changes", res)
	}

	// Simulate the drag the tree would deliver after capture.
	gap := Profile().DensityValues().MobileWheelTickGap
	h.OnDrag(rx, y0-gap*3) // drag up = finer resolution
	h.OnRelease(rx, y0-gap*3)

	if k.StepMul == startStep {
		t.Fatalf("resolution drag should change StepMul from %v (captured drag never reached applyResDelta)", startStep)
	}
	if k.StepMul != badge.Step() {
		t.Fatalf("knob StepMul %v out of sync with badge.Step() %v", k.StepMul, badge.Step())
	}
}

func TestWheelPortalOverlayHitAreasAndClose(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0)
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(5, 5, 15, 15), image.Rect(0, 0, 400, 800), 0)

	o := &mobileWheelPopupPortalOverlay{popup: w, tag: "synth-wheel-popup"}
	areas := o.HitAreas()
	if len(areas) == 0 {
		t.Fatal("open wheel must publish at least one hit area")
	}
	if o.ShouldClose() {
		t.Fatal("ShouldClose must be false while open")
	}
	w.Close()
	if !o.ShouldClose() {
		t.Fatal("ShouldClose must be true after Close")
	}
}
