// src/go/internal/ui/mobile_wheel_resolution_test.go
package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestWheelResolutionDragChangesStepMul(t *testing.T) {
	def := audio.ParamDef{Name: "cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	k := &Knob{Endless: true}
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Unit: def.Unit}
	k.Value = 0.5
	badge := NewKnobStepBadge(def)
	k.StepMul = badge.Step()
	startStep := k.StepMul

	resolved := 0
	b := WheelBinding{Knob: k, Badge: badge, Def: def,
		OnChange: func() {}, OnResolution: func() { resolved++ }, Title: func() string { return "Cutoff" }}
	w := NewMobileWheelPopup()
	w.Open(b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	rx := w.resRect.Min.X + 2
	y0 := w.resRect.Min.Y + w.resRect.Dy()/2
	gap := Profile().DensityValues().MobileWheelTickGap
	w.HandleInput(rx, y0, true)
	w.HandleInput(rx, y0-gap*3, true) // drag up = finer, several rungs
	w.HandleInput(rx, y0-gap*3, false)

	if k.StepMul == startStep {
		t.Fatalf("resolution drag should change StepMul from %v", startStep)
	}
	if k.StepMul != badge.Step() {
		t.Fatalf("knob StepMul %v out of sync with badge.Step() %v", k.StepMul, badge.Step())
	}
	if resolved == 0 {
		t.Fatal("OnResolution callback never fired")
	}
}

func TestWheelEnumStepsLabels(t *testing.T) {
	def := audio.ParamDef{Name: "wave", Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Tri"}}
	k := &Knob{Discrete: true}
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Enum: def.Enum}
	k.Value = 0 // Sine
	b := WheelBinding{Knob: k, Def: def, Discrete: true,
		OnChange: func() {}, OnCommit: func() {}, Title: func() string { return "Wave" }}
	w := NewMobileWheelPopup()
	w.Open(b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	// no resolution strip for enums
	if !w.resRect.Empty() {
		t.Fatal("enum wheel must not show a resolution strip")
	}

	cx := w.valRect.Min.X + 4
	y0 := w.valRect.Min.Y + w.valRect.Dy()/2
	gap := Profile().DensityValues().MobileWheelTickGap
	w.HandleInput(cx, y0, true)
	w.HandleInput(cx, y0+gap, true) // one detent worth, drag down
	w.HandleInput(cx, y0+gap, false)

	if k.Value == 0 {
		t.Fatalf("enum wheel drag should advance the detent; value still %v", k.Value)
	}
}

func TestWheelHandleWheelChangesValue(t *testing.T) {
	k := newEndlessKnobForTest(0, 200, 1.0) // value 0.5 -> real 100
	b, _ := wheelTestBinding(k)
	w := NewMobileWheelPopup()
	w.Open(*b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	// Scroll up (positive steps) over the barrel -> value increases. Scroll is
	// paced, so issue a full notch of events.
	cx := w.valRect.Min.X + 4
	cy := (w.valRect.Min.Y + w.valRect.Max.Y) / 2
	start := realValue(k)
	if !w.HandleWheel(cx, cy, 5) {
		t.Fatal("HandleWheel should consume a barrel scroll")
	}
	for i := 0; i < wheelStepEventsPerNotch; i++ {
		w.HandleWheel(cx, cy, 5)
	}
	if realValue(k) <= start {
		t.Fatalf("wheel up should increase value from %v, got %v", start, realValue(k))
	}
	// Scroll down -> decreases.
	mid := realValue(k)
	for i := 0; i < wheelStepEventsPerNotch+1; i++ {
		w.HandleWheel(cx, cy, -8)
	}
	if realValue(k) >= mid {
		t.Fatalf("wheel down should decrease value from %v, got %v", mid, realValue(k))
	}
}

func TestWheelHandleWheelOverStripChangesResolution(t *testing.T) {
	def := audio.ParamDef{Name: "cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	k := &Knob{Endless: true}
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Unit: def.Unit}
	k.Value = 0.5
	badge := NewKnobStepBadge(def)
	k.StepMul = badge.Step()
	start := k.StepMul
	b := WheelBinding{Knob: k, Badge: badge, Def: def, OnChange: func() {}, OnResolution: func() {}, Title: func() string { return "Cutoff" }}
	w := NewMobileWheelPopup()
	w.Open(b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	rx := w.resRect.Min.X + 2
	ry := (w.resRect.Min.Y + w.resRect.Max.Y) / 2
	if !w.HandleWheel(rx, ry, 3) {
		t.Fatal("HandleWheel over the resolution strip should consume")
	}
	// Paced: issue a full notch of scroll events to shift one rung.
	for i := 0; i < wheelStepEventsPerNotch; i++ {
		w.HandleWheel(rx, ry, 3)
	}
	if k.StepMul == start {
		t.Fatalf("wheel over strip should change StepMul from %v", start)
	}
	if k.StepMul != badge.Step() {
		t.Fatalf("knob StepMul %v out of sync with badge %v", k.StepMul, badge.Step())
	}
}
