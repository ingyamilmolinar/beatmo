//go:build test

package ui

import (
	"testing"
)

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

// canStep refuses to go below Min, so a row below it would be a lie: labelAt returns "" there.
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

// A saved value off the active grid must not round adjacent rows to one label, which reads as a frozen wheel.
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
		// Half a step above Min, the worst case for a precision derived from the step.
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
