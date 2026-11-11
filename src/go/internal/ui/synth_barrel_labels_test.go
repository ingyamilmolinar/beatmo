//go:build test

package ui

import (
	"testing"
)

// armedContinuousControl arms the first continuous synth control that has a
// resolution ladder, and returns its value binding plus the live knob.
func armedContinuousControl(t *testing.T) (synthValueBinding, *Knob, float64) {
	t.Helper()
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()

	for idx := range dv.instEditorControls {
		k := dv.instEditorControls[idx]
		if k == nil || k.Discrete || idx >= len(dv.instEditorStepBadges) {
			continue
		}
		badge := dv.instEditorStepBadges[idx]
		if badge == nil || idx >= len(dv.instEditorBindings) {
			continue
		}
		def := dv.instEditorBindings[idx].def
		if def.Max <= def.Min || len(def.Enum) > 0 {
			continue
		}
		dv.armSynthControl(inst, idx)
		k.StepMul = badge.Step()
		return synthValueBinding{dv: dv, idx: idx, instID: inst}, k, badge.Step()
	}
	t.Skip("no continuous synth control with a step ladder in this instrument")
	return synthValueBinding{}, nil, 0
}

// TestBarrelValueLabelsStopAtRangeEnd proves the barrel never advertises a value
// the wheel cannot reach. canStep() already refuses to move below Min, so a row
// reading "-0.01 s" under a 0-minimum Attack is a lie; labelAt must return ""
// there and drawBarrelLane skips it.
func TestBarrelValueLabelsStopAtRangeEnd(t *testing.T) {
	b, k, _ := armedContinuousControl(t)
	def, _ := b.def()

	k.Value = 0 // parked at def.Min
	if got := b.labelAt(-1); got != "" {
		t.Errorf("row below Min (%v) must not be labelled, got %q", def.Min, got)
	}
	if got := b.labelAt(0); got == "" {
		t.Error("the center row must always be labelled")
	}

	k.Value = 1 // parked at def.Max
	if got := b.labelAt(+1); got != "" {
		t.Errorf("row above Max (%v) must not be labelled, got %q", def.Max, got)
	}
}

// TestBarrelValueLabelsDistinctOffGrid proves adjacent rows never format to the
// same string. A saved param value is under no obligation to land on the active
// resolution's grid; parked half a step off it, rows one detent apart used to
// round to an identical label (0.005 and 0.015 both rendering "0.01 s"), which
// reads as a frozen wheel.
func TestBarrelValueLabelsDistinctOffGrid(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()

	checked := 0
	for idx := range dv.instEditorControls {
		k := dv.instEditorControls[idx]
		if k == nil || k.Discrete || idx >= len(dv.instEditorStepBadges) ||
			idx >= len(dv.instEditorBindings) {
			continue
		}
		badge := dv.instEditorStepBadges[idx]
		if badge == nil {
			continue
		}
		def := dv.instEditorBindings[idx].def
		span := def.Max - def.Min
		if span <= 0 || len(def.Enum) > 0 {
			continue
		}
		step := badge.Step()
		k.StepMul = step
		// Park exactly half a step above Min — the worst case for a display
		// precision derived from the step alone.
		k.Value = (0.5 * step) / span

		b := synthValueBinding{dv: dv, idx: idx, instID: inst}
		center, above := b.labelAt(0), b.labelAt(+1)
		if center == "" || above == "" {
			t.Errorf("%s: center/above rows must be labelled, got %q / %q", def.Name, center, above)
			continue
		}
		if center == above {
			t.Errorf("%s: adjacent rows must be distinguishable off-grid: both read %q (step %v, span %v)",
				def.Name, center, step, span)
		}
		checked++
	}
	if checked == 0 {
		t.Skip("no continuous synth control with a step ladder in this instrument")
	}
}
