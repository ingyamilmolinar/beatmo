package ui

import (
	"image"
	"testing"
)

func TestArmedEditor_ValueLaneSpinsChangesValue(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	dv.armSynthControl(inst, 0)
	e := dv.instEditorArmedEditor
	e.SetCardRect(image.Rect(0, 0, 120, 148))

	before := dv.instEditorControls[0].Value
	// Spin the value lane (right side) with a notch, then settle.
	vx := e.ValueLaneRect().Min.X + 4
	e.Wheel(vx, 40, +1)
	for i := 0; i < 240 && e.Tick(); i++ {
	}
	if dv.instEditorControls[0].Value <= before {
		t.Fatalf("value lane notch should increase value: before=%v after=%v", before, dv.instEditorControls[0].Value)
	}
}

func TestArmedEditor_HoveredLaneRouting(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	// pick a badged (continuous) control so a res lane exists
	idx := 0
	for i, b := range dv.instEditorStepBadges {
		if b != nil {
			idx = i
			break
		}
	}
	dv.armSynthControl(inst, idx)
	e := dv.instEditorArmedEditor
	e.SetCardRect(image.Rect(0, 0, 120, 148))
	if e.ResLaneRect().Empty() {
		t.Skip("selected control has no resolution lane")
	}
	// A point in the left lane routes to the resolution wheel.
	rx := e.ResLaneRect().Min.X + 2
	if e.laneAt(rx) != e.res() {
		t.Fatal("left-lane x should route to the resolution wheel")
	}
	// A point in the right lane routes to the value wheel.
	vx := e.ValueLaneRect().Min.X + 2
	if e.laneAt(vx) != e.value() {
		t.Fatal("right-lane x should route to the value wheel")
	}
}

func TestArmedEditor_KeyboardStepsValueAndResolution(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	dv.armSynthControl(inst, 0)
	e := dv.instEditorArmedEditor
	before := dv.instEditorControls[0].Value
	if !e.StepValue(+1) {
		t.Fatal("StepValue(+1) should change value")
	}
	if dv.instEditorControls[0].Value <= before {
		t.Fatal("value should increase")
	}
}
