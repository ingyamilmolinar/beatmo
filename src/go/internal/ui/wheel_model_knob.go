package ui

import "github.com/ingyamilmolinar/beatmo/internal/audio"

// Reproduces the wheel's historical semantics exactly (TestWheelCharacterizationTape); every ladder move keeps
// Knob.StepMul in sync with the badge.
type knobModel struct {
	k        *Knob
	b        *KnobStepBadge // nil = no resolution ladder
	def      audio.ParamDef
	discrete bool
}

func newKnobModel(k *Knob, b *KnobStepBadge, def audio.ParamDef, discrete bool) WheelModel {
	return &knobModel{k: k, b: b, def: def, discrete: discrete}
}

func (m *knobModel) Snapshot() float64 {
	if m.k == nil {
		return 0
	}
	return m.k.Value
}

func (m *knobModel) Restore(v float64) {
	if m.k != nil {
		m.k.Value = v
	}
}

// StepMul, else span/200; shared by the readout and the row walk.
func (m *knobModel) step() float64 { return knobStepResolution(m.k, m.def) }

func (m *knobModel) CenterLabel() string {
	if m.k == nil {
		return ""
	}
	return formatKnobDisplayStep(m.def, m.k.Value, m.step())
}

func (m *knobModel) RowLabel(offset int) (string, bool) {
	if m.k == nil || offset == 0 {
		return "", false
	}
	if cnt := len(m.def.Enum); cnt > 0 {
		idx := enumIndex(m.k.Value, cnt) + offset
		if idx < 0 || idx >= cnt {
			return "", false
		}
		return m.def.Enum[idx], true
	}
	_, lbl, ok := m.walk(offset)
	return lbl, ok
}

// Walks one StepMul at a time until the label changes, |offset| times: the distinct-label sequence the barrel draws.
func (m *knobModel) walk(offset int) (float64, string, bool) {
	def := m.def
	span := def.Max - def.Min
	step := m.step()
	if span <= 0 || step <= 0 {
		return 0, "", false
	}
	dir := 1
	n := offset
	if n < 0 {
		dir, n = -1, -n
	}
	val := def.Min + m.k.Value*span
	prev := formatSynthParamValueStep(def, val, step)
	for r := 0; r < n; r++ {
		advanced := false
		for guard := 0; guard < 4096; guard++ {
			val += float64(dir) * step
			if val < def.Min || val > def.Max {
				return 0, "", false
			}
			if cand := formatSynthParamValueStep(def, val, step); cand != prev {
				prev, advanced = cand, true
				break
			}
		}
		if !advanced {
			return 0, "", false
		}
	}
	return val, prev, true
}

func (m *knobModel) SelectRow(offset int) bool {
	if m.k == nil || offset == 0 {
		return false
	}
	if cnt := len(m.def.Enum); cnt > 0 {
		idx := enumIndex(m.k.Value, cnt) + offset
		if idx < 0 || idx >= cnt {
			return false
		}
		if cnt == 1 {
			m.k.Value = 0
		} else {
			m.k.Value = float64(idx) / float64(cnt-1)
		}
		return true
	}
	val, _, ok := m.walk(offset)
	if !ok {
		return false
	}
	m.k.Value = (val - m.def.Min) / (m.def.Max - m.def.Min)
	return true
}

func (m *knobModel) Step(n int) bool {
	if m.k == nil || n == 0 {
		return false
	}
	before := m.k.Value
	if m.discrete {
		applyKnobSteps(m.k, n)
	} else {
		// knobEndlessPxPerNotch is exactly one StepMul.
		m.k.NudgeEndless(n * knobEndlessPxPerNotch)
	}
	return m.k.Value != before
}

func (m *knobModel) StepFine(n int) bool {
	if m.k == nil || n == 0 {
		return false
	}
	if m.discrete || m.b == nil {
		return m.Step(n)
	}
	i := badgeIndexBelow(m.b)
	if i < 0 || i >= len(m.b.Rungs()) {
		return m.Step(n)
	}
	saved := m.k.StepMul
	m.k.StepMul = m.b.Rungs()[i]
	defer func() { m.k.StepMul = saved }()
	return m.Step(n)
}

func (m *knobModel) HasLadder() bool { return m.b != nil && !m.discrete }

func (m *knobModel) LadderLen() int {
	if !m.HasLadder() {
		return 0
	}
	return m.b.LadderLen()
}

func (m *knobModel) LadderIndex() int {
	if m.b == nil {
		return 0
	}
	return m.b.Index()
}

func (m *knobModel) LadderLabel(i int) string {
	if m.b == nil || i < 0 || i >= len(m.b.Rungs()) {
		return ""
	}
	return formatStepValue(m.def, m.b.Rungs()[i])
}

func (m *knobModel) syncStep() {
	if m.k != nil && m.b != nil {
		m.k.StepMul = m.b.Step()
	}
}

func (m *knobModel) SetLadderIndex(i int) bool {
	if m.b == nil || !m.b.SetIndex(i) {
		return false
	}
	m.syncStep()
	return true
}

func (m *knobModel) AdjustLadder(d int) bool {
	if m.b == nil || !m.b.AdjustResolution(d) {
		return false
	}
	m.syncStep()
	return true
}
