//go:build test

package ui

import (
	"image"
	"strconv"
	"testing"
)

func intModel(min, max, v float64, ladder ...float64) *NumberModel {
	return &NumberModel{
		Min: min, Max: max, Value: v, Ladder: ladder,
		Format: func(v, step float64) string { return strconv.Itoa(int(v)) },
	}
}

func TestNumberModelStepClampsAtBothEnds(t *testing.T) {
	m := intModel(1, 10, 9, 1)
	if !m.Step(1) || m.Value != 10 {
		t.Fatalf("step to max: %v", m.Value)
	}
	if m.Step(1) || m.Value != 10 {
		t.Fatalf("step past max must clamp and report no change: %v", m.Value)
	}
	m.Value = 2
	if !m.Step(-5) || m.Value != 1 {
		t.Fatalf("big step below min must clamp to min: %v", m.Value)
	}
	if m.Step(-1) {
		t.Fatalf("step below min must report no change")
	}
}

func TestNumberModelSnapsToTheRungGrid(t *testing.T) {
	// Coarse steps snap to grid lines: 13 + one detent of 4 is 16, not 17.
	m := intModel(0, 100, 13, 1, 4)
	m.Rung = 1
	m.Step(1)
	if m.Value != 16 {
		t.Errorf("up from 13 at step 4 = %v, want 16", m.Value)
	}
	m.Value = 13
	m.Step(-1)
	if m.Value != 12 {
		t.Errorf("down from 13 at step 4 = %v, want 12", m.Value)
	}
	m.Value = 12
	m.Step(1)
	if m.Value != 16 {
		t.Errorf("up from on-grid 12 = %v, want 16", m.Value)
	}
}

func TestNumberModelRowLabelsFollowTheRung(t *testing.T) {
	m := intModel(0, 20, 10, 1, 5)
	if l, ok := m.RowLabel(1); !ok || l != "11" {
		t.Errorf("rung 1: row +1 = %q %v", l, ok)
	}
	m.Rung = 1
	if l, ok := m.RowLabel(2); !ok || l != "20" {
		t.Errorf("rung 5: row +2 = %q %v", l, ok)
	}
	if _, ok := m.RowLabel(3); ok {
		t.Errorf("rung 5: row +3 (25) must be out of range")
	}
	if l, ok := m.RowLabel(-2); !ok || l != "0" {
		t.Errorf("rung 5: row -2 = %q %v", l, ok)
	}
}

func TestNumberModelSelectRowMatchesItsLabel(t *testing.T) {
	m := intModel(0, 100, 37, 1, 4, 16)
	m.Rung = 1
	want, _ := m.RowLabel(-3)
	if !m.SelectRow(-3) {
		t.Fatal("SelectRow refused")
	}
	if got := m.CenterLabel(); got != want {
		t.Errorf("centre %q, row read %q", got, want)
	}
}

func TestNumberModelLadder(t *testing.T) {
	m := intModel(0, 100, 0, 1, 4, 16)
	var persisted []int
	m.OnRung = func(i int) { persisted = append(persisted, i) }
	if !m.HasLadder() || m.LadderLen() != 3 {
		t.Fatalf("ladder: has=%v len=%d", m.HasLadder(), m.LadderLen())
	}
	if m.AdjustLadder(1) { // +1 = finer; already finest
		t.Errorf("AdjustLadder(+1) at finest must refuse")
	}
	if !m.AdjustLadder(-1) || m.LadderIndex() != 1 {
		t.Errorf("AdjustLadder(-1) = coarser, at %d", m.LadderIndex())
	}
	if !m.SetLadderIndex(2) || m.LadderIndex() != 2 || m.SetLadderIndex(5) {
		t.Errorf("SetLadderIndex bounds")
	}
	if len(persisted) != 2 {
		t.Errorf("OnRung fired %d times, want 2", len(persisted))
	}
	if intModel(0, 1, 0, 1).HasLadder() || intModel(0, 1, 0).HasLadder() {
		t.Errorf("a single-rung or empty ladder shows no ladder")
	}
}

func TestNumberModelStepFineUsesTheNextFinerRung(t *testing.T) {
	m := intModel(0, 100, 40, 1, 4, 16)
	m.Rung = 2
	m.StepFine(1)
	if m.Value != 44 || m.Rung != 2 {
		t.Errorf("fine step from 40 at rung 16 = %v (rung %d), want 44 at rung 2", m.Value, m.Rung)
	}
	m.Rung = 0
	m.StepFine(1)
	if m.Value != 45 {
		t.Errorf("fine at finest rung = normal step: %v", m.Value)
	}
}

func TestNumberModelSnapshotRestore(t *testing.T) {
	m := intModel(0, 10, 5, 1)
	s := m.Snapshot()
	m.Step(3)
	m.Restore(s)
	if m.Value != 5 {
		t.Errorf("restore: %v", m.Value)
	}
}

func TestNumberModelGridIsMultiplesOfTheStepNotOffsetByMin(t *testing.T) {
	m := intModel(1, 1000, 120, 5)
	m.Step(1)
	if m.Value != 125 {
		t.Errorf("BPM 120 + one step of 5 with Min 1 = %v, want 125", m.Value)
	}
	m.Value = 3
	m.Step(-1)
	if m.Value != 1 {
		t.Errorf("below the first grid line clamps to Min: %v", m.Value)
	}
}

func TestNumberModelFloatStepsDoNotDrift(t *testing.T) {
	m := &NumberModel{Min: 0, Max: 1, Value: 0, Ladder: []float64{0.1},
		Format: func(v, step float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }}
	for i := 0; i < 10; i++ {
		m.Step(1)
	}
	if m.Value != 1 {
		t.Errorf("ten 0.1 steps from 0 = %v, want exactly 1", m.Value)
	}
	if !m.Step(-3) || m.CenterLabel() != "0.7" {
		t.Errorf("after -3: %v", m.Value)
	}
}

// Live on drag and arrows, Esc restores, Enter commits once, exactly like a knob.
func TestWheelPopupDrivesANumberModel(t *testing.T) {
	m := intModel(1, 1000, 120, 1, 5, 10, 50)
	changes, commits := 0, 0
	w := NewMobileWheelPopup()
	w.Open(WheelBinding{
		Model:    m,
		Title:    func() string { return "BPM" },
		OnChange: func() { changes++ },
		OnCommit: func() { commits++ },
	}, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	if !w.hasResolutionStrip() || len(w.rungRects()) != 4 {
		t.Fatalf("a 4-rung NumberModel must show its ladder (rungs %d)", len(w.rungRects()))
	}
	dragWheelValue(w, w.tickGap()*2) // finger UP two notches = value DOWN
	if m.Value >= 120 || changes == 0 {
		t.Fatalf("drag up must lower the value live: %v (changes %d)", m.Value, changes)
	}
	if !w.StepValue(+1) {
		t.Fatalf("StepValue(+1) refused")
	}
	if !w.StepResolution(+1) || m.Rung != 1 {
		t.Fatalf("StepResolution(+1) = coarser, rung %d", m.Rung)
	}
	w.Cancel()
	if m.Value != 120 || commits != 0 {
		t.Fatalf("Esc must restore 120 without committing: %v, commits %d", m.Value, commits)
	}
	w.Open(WheelBinding{Model: m, Title: func() string { return "BPM" },
		OnChange: func() {}, OnCommit: func() { commits++ }},
		image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	w.StepValue(+1)
	w.Accept()
	if m.Value != 125 || commits != 1 {
		t.Fatalf("Enter: value %v (want 125 at rung 5), commits %d", m.Value, commits)
	}
}

// Rows wrap too, so the barrel never shows an empty slot.
func TestNumberModelWrap(t *testing.T) {
	m := intModel(0, 23, 23, 1)
	m.Wrap = true
	if !m.Step(1) || m.Value != 0 {
		t.Fatalf("23 + 1 wraps to 0, got %v", m.Value)
	}
	if !m.Step(-1) || m.Value != 23 {
		t.Fatalf("0 - 1 wraps to 23, got %v", m.Value)
	}
	if l, ok := m.RowLabel(2); !ok || l != "1" {
		t.Fatalf("row +2 from 23 = %q %v, want 1", l, ok)
	}
	if !m.SelectRow(2) || m.Value != 1 {
		t.Fatalf("SelectRow(+2) from 23 = %v", m.Value)
	}
}
