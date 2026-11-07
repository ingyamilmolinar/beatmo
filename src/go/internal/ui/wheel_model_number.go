package ui

import "math"

// A detent moves to the NEXT line of the step's grid (multiples of the step: 120 → 125 → 130 at 5), so a coarse
// step from off-grid snaps first: 4 beats from 13 lands on 16. Ladder runs fine→coarse.
type NumberModel struct {
	Min, Max, Value float64
	Ladder          []float64
	Rung            int
	// The step lets a label show only the precision the resolution has.
	Format func(v, step float64) string
	// LadderFormat renders a rung's step for the ladder strip; nil = Format.
	LadderFormat func(step float64) string
	// OnRung, when set, is told about every rung change (persistence).
	OnRung func(i int)
	// Clock fields roll over (23 → 0); the period is Max - Min + one finest step.
	Wrap bool
}

func (m *NumberModel) Snapshot() float64   { return m.Value }
func (m *NumberModel) Restore(v float64)   { m.Value = m.clamp(v) }
func (m *NumberModel) CenterLabel() string { return m.format(m.Value) }

// finest is the smallest ladder step (1 when there is no ladder).
func (m *NumberModel) finest() float64 {
	if len(m.Ladder) > 0 && m.Ladder[0] > 0 {
		return m.Ladder[0]
	}
	return 1
}

func (m *NumberModel) step() float64 {
	if m.Rung >= 0 && m.Rung < len(m.Ladder) && m.Ladder[m.Rung] > 0 {
		return m.Ladder[m.Rung]
	}
	return 1
}

func (m *NumberModel) format(v float64) string {
	if m.Format == nil {
		return trimFloat(v)
	}
	return m.Format(v, m.step())
}

func (m *NumberModel) clamp(v float64) float64 {
	if m.Wrap {
		period := m.Max - m.Min + m.finest()
		if period > 0 {
			v = m.Min + math.Mod(v-m.Min, period)
			if v < m.Min {
				v += period
			}
			if v > m.Max+1e-9 { // within the last step of the period
				v = m.Min
			}
			return v
		}
	}
	if v < m.Min {
		return m.Min
	}
	if v > m.Max {
		return m.Max
	}
	return v
}

// The first line each way is strictly beyond v; k·s from an integer k, so repeated steps never drift.
func (m *NumberModel) gridLine(v, s float64, n int) float64 {
	k := v / s
	const eps = 1e-9
	var base float64
	if n > 0 {
		base = math.Floor(k + eps)
	} else {
		base = math.Ceil(k - eps)
	}
	return (base + float64(n)) * s
}

// rowValue is the value offset rows from the current one, ok=false past a bound.
func (m *NumberModel) rowValue(offset int) (float64, bool) {
	if offset == 0 {
		return m.Value, true
	}
	v := m.gridLine(m.Value, m.step(), offset)
	if m.Wrap {
		return m.clamp(v), true
	}
	const eps = 1e-9
	if v < m.Min-eps || v > m.Max+eps {
		return 0, false
	}
	return m.clamp(v), true
}

func (m *NumberModel) RowLabel(offset int) (string, bool) {
	if offset == 0 {
		return "", false
	}
	v, ok := m.rowValue(offset)
	if !ok {
		return "", false
	}
	return m.format(v), true
}

func (m *NumberModel) SelectRow(offset int) bool {
	if offset == 0 {
		return false
	}
	v, ok := m.rowValue(offset)
	if !ok {
		return false
	}
	m.Value = v
	return true
}

func (m *NumberModel) stepBy(n int, s float64) bool {
	if n == 0 {
		return false
	}
	before := m.Value
	m.Value = m.clamp(m.gridLine(m.Value, s, n))
	return m.Value != before
}

func (m *NumberModel) Step(n int) bool { return m.stepBy(n, m.step()) }

func (m *NumberModel) StepFine(n int) bool {
	if m.Rung > 0 && m.Rung-1 < len(m.Ladder) && m.Ladder[m.Rung-1] > 0 {
		return m.stepBy(n, m.Ladder[m.Rung-1])
	}
	return m.Step(n)
}

func (m *NumberModel) HasLadder() bool { return len(m.Ladder) > 1 }

func (m *NumberModel) LadderLen() int {
	if !m.HasLadder() {
		return 0
	}
	return len(m.Ladder)
}

func (m *NumberModel) LadderIndex() int { return m.Rung }

func (m *NumberModel) LadderLabel(i int) string {
	if i < 0 || i >= len(m.Ladder) {
		return ""
	}
	if m.LadderFormat != nil {
		return m.LadderFormat(m.Ladder[i])
	}
	return m.format(m.Ladder[i])
}

func (m *NumberModel) SetLadderIndex(i int) bool {
	if i < 0 || i >= len(m.Ladder) || i == m.Rung {
		return false
	}
	m.Rung = i
	if m.OnRung != nil {
		m.OnRung(i)
	}
	return true
}

func (m *NumberModel) AdjustLadder(d int) bool {
	// +1 = finer (KnobStepBadge semantics) = a LOWER rung index.
	return m.SetLadderIndex(m.Rung - d)
}
