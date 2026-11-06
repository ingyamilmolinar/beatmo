//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// knobModel is the WheelModel adapter that keeps the Sampler / EQ wheels on
// their existing Knob + KnobStepBadge semantics. These tests pin the contract
// the ValueWheel view relies on; the byte-exact behaviour is pinned separately
// by TestWheelCharacterizationTape.

func TestKnobModelSelectRowLandsOnTheLabelItShows(t *testing.T) {
	def := audio.ParamDef{Name: "eq_band_gain", Min: -12, Max: 12, Unit: "dB"}
	for _, step := range []float64{0.05, 0.1, 1, 3} {
		for _, n := range []int{-3, -1, 1, 2, 4} {
			k := newEndlessKnobForTest(-12, 12, step)
			m := newKnobModel(k, nil, def, false)
			want, ok := m.RowLabel(n)
			if !ok {
				t.Fatalf("step %v: row %d should be in range", step, n)
			}
			if !m.SelectRow(n) {
				t.Fatalf("step %v: SelectRow(%d) refused", step, n)
			}
			if got := m.CenterLabel(); got != want {
				t.Errorf("step %v row %d: centre now %q, the row read %q", step, n, got, want)
			}
		}
	}
}

func TestKnobModelRowLabelsAreDistinctAndOrdered(t *testing.T) {
	def := audio.ParamDef{Name: "x", Min: 0, Max: 100, Unit: "%"}
	k := newEndlessKnobForTest(0, 100, 0.001) // finer than the label precision
	m := newKnobModel(k, nil, def, false)
	seen := map[string]bool{m.CenterLabel(): true}
	for n := 1; n <= 4; n++ {
		up, ok := m.RowLabel(n)
		if !ok {
			t.Fatalf("row %d out of range", n)
		}
		if seen[up] {
			t.Errorf("row %d repeats label %q", n, up)
		}
		seen[up] = true
	}
}

func TestKnobModelRowLabelOutOfRange(t *testing.T) {
	def := audio.ParamDef{Name: "x", Min: 0, Max: 10}
	k := newEndlessKnobForTest(0, 10, 1)
	k.Value = 1 // at Max
	m := newKnobModel(k, nil, def, false)
	if _, ok := m.RowLabel(1); ok {
		t.Errorf("a row above Max must be out of range")
	}
	if _, ok := m.RowLabel(-1); !ok {
		t.Errorf("a row below Max must exist")
	}
}

func TestKnobModelEnumRows(t *testing.T) {
	def := audio.ParamDef{Name: "wave", Enum: []string{"Sine", "Saw", "Square", "Triangle"}}
	k := &Knob{Value: 1.0 / 3.0, Discrete: true} // index 1
	k.Scale = KnobScale{Enum: def.Enum}
	m := newKnobModel(k, nil, def, true)
	if got, _ := m.RowLabel(1); got != "Square" {
		t.Errorf("RowLabel(1) = %q, want Square", got)
	}
	if got, _ := m.RowLabel(-1); got != "Sine" {
		t.Errorf("RowLabel(-1) = %q, want Sine", got)
	}
	if _, ok := m.RowLabel(-2); ok {
		t.Errorf("RowLabel(-2) must be out of range")
	}
	if m.HasLadder() {
		t.Errorf("a discrete knob has no resolution ladder")
	}
	if !m.Step(1) || enumIndex(k.Value, 4) != 2 {
		t.Errorf("Step(1) must advance one enum item, at %d", enumIndex(k.Value, 4))
	}
}

func TestKnobModelStepReportsClampAsNoChange(t *testing.T) {
	def := audio.ParamDef{Name: "x", Min: 0, Max: 10}
	k := newEndlessKnobForTest(0, 10, 1)
	k.Value = 1
	m := newKnobModel(k, nil, def, false)
	if m.Step(1) {
		t.Errorf("Step past Max must report no change")
	}
	if !m.Step(-1) {
		t.Errorf("Step below Max must report a change")
	}
}

func TestKnobModelLadderKeepsStepMulInSync(t *testing.T) {
	def := audio.ParamDef{Name: "eq_band_gain", Min: -12, Max: 12, Unit: "dB"}
	b := NewKnobStepBadge(def)
	k := newEndlessKnobForTest(-12, 12, b.Step())
	m := newKnobModel(k, b, def, false)
	if !m.HasLadder() || m.LadderLen() != b.LadderLen() {
		t.Fatalf("ladder not exposed: has=%v len=%d", m.HasLadder(), m.LadderLen())
	}
	start := m.LadderIndex()
	if !m.AdjustLadder(1) { // Badge semantics: +1 = finer
		t.Fatalf("AdjustLadder(+1) refused at rung %d", start)
	}
	if k.StepMul != b.Step() {
		t.Errorf("StepMul %v not synced to badge step %v", k.StepMul, b.Step())
	}
	if !m.SetLadderIndex(0) || k.StepMul != b.Rungs()[0] {
		t.Errorf("SetLadderIndex(0) must sync StepMul to rung 0")
	}
	if m.LadderLabel(0) != formatStepValue(def, b.Rungs()[0]) {
		t.Errorf("LadderLabel(0) = %q", m.LadderLabel(0))
	}
}

func TestKnobModelSnapshotRestore(t *testing.T) {
	def := audio.ParamDef{Name: "x", Min: 0, Max: 10}
	k := newEndlessKnobForTest(0, 10, 1)
	m := newKnobModel(k, nil, def, false)
	snap := m.Snapshot()
	m.Step(3)
	m.Restore(snap)
	if k.Value != snap {
		t.Errorf("Restore: %v want %v", k.Value, snap)
	}
}
