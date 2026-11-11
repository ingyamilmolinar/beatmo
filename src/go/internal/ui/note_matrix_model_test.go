//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The matrix must agree with the note label the sidebar and coord badge
// already show — all three derive from the same A3 = pitch-0 convention. If
// this drifts, tapping a cell labelled "C4" would set a pitch that reads back
// as something else.

func TestNoteMatrix_PitchZeroIsA3(t *testing.T) {
	m := newNoteMatrixModel(0)
	col, row, ok := m.CellFor(0)
	if !ok {
		t.Fatal("pitch 0 has no cell")
	}
	if got := m.NoteLabelAt(col, row); got != "A3" {
		t.Errorf("pitch 0 cell = %q, want A3 (the engine's pitch-0 convention)", got)
	}
}

// The instrument's own offset shifts every label, exactly as it does in
// nodeNoteLabel: a +3 instrument puts node pitch 0 at C4.
func TestNoteMatrix_InstrumentOffsetShiftsLabels(t *testing.T) {
	m := newNoteMatrixModel(3)
	col, row, ok := m.CellFor(0)
	if !ok {
		t.Fatal("pitch 0 has no cell")
	}
	if got := m.NoteLabelAt(col, row); got != "C4" {
		t.Errorf("pitch 0 with a +3 instrument = %q, want C4", got)
	}
}

// Round-trip: every selectable cell must map to a pitch that maps back to it.
func TestNoteMatrix_CellPitchRoundTrip(t *testing.T) {
	for _, offset := range []float64{-7, 0, 3, 12} {
		m := newNoteMatrixModel(offset)
		selectable := 0
		for col := 0; col < m.Cols(); col++ {
			for row := 0; row < noteMatrixRows; row++ {
				pitch, ok := m.PitchAt(col, row)
				if !ok {
					continue
				}
				selectable++
				gc, gr, ok := m.CellFor(pitch)
				if !ok {
					t.Errorf("offset %v: pitch %v from cell (%d,%d) maps to no cell", offset, pitch, col, row)
					continue
				}
				if gc != col || gr != row {
					t.Errorf("offset %v: cell (%d,%d) -> pitch %v -> cell (%d,%d)", offset, col, row, pitch, gc, gr)
				}
			}
		}
		// The range is inclusive on both ends, so it is 49 semitones.
		if want := int(model.NodePitchMax-model.NodePitchMin) + 1; selectable != want {
			t.Errorf("offset %v: %d selectable cells, want %d (the full ±%v range)",
				offset, selectable, want, model.NodePitchMax)
		}
	}
}

// Cells outside the range must be rejected, not clamped — clamping would make
// two different cells select the same pitch.
func TestNoteMatrix_OutOfRangeCellsAreNotSelectable(t *testing.T) {
	m := newNoteMatrixModel(0)
	rejected := 0
	for col := 0; col < m.Cols(); col++ {
		for row := 0; row < noteMatrixRows; row++ {
			if _, ok := m.PitchAt(col, row); !ok {
				rejected++
			}
		}
	}
	if rejected == 0 {
		t.Error("no cell was rejected; the grid is a rectangle but the range is a run, so some corners must fall outside")
	}
	if _, ok := m.PitchAt(-1, 0); ok {
		t.Error("negative column was accepted")
	}
	if _, ok := m.PitchAt(0, noteMatrixRows); ok {
		t.Error("row past the last was accepted")
	}
}

// A pitch outside the matrix must report no selection rather than a wrong one.
func TestNoteMatrix_PitchOutsideRangeHasNoCell(t *testing.T) {
	m := newNoteMatrixModel(0)
	if _, _, ok := m.CellFor(model.NodePitchMax + 1); ok {
		t.Error("a pitch above the range reported a cell")
	}
	if _, _, ok := m.CellFor(model.NodePitchMin - 1); ok {
		t.Error("a pitch below the range reported a cell")
	}
}

// Pitch increases upward and rightward.
func TestNoteMatrix_HigherPitchIsUpAndRight(t *testing.T) {
	m := newNoteMatrixModel(0)
	// Within a column, the row above sounds higher.
	if m.MIDIAt(1, 0) <= m.MIDIAt(1, 1) {
		t.Error("row 0 must sound higher than row 1 (larger numbers on top)")
	}
	// Across columns, the next column to the right is an octave up.
	if got := m.MIDIAt(2, 5) - m.MIDIAt(1, 5); got != 12 {
		t.Errorf("adjacent columns differ by %d semitones, want 12 (one octave)", got)
	}
}

func TestNoteMatrix_RowAndColumnLabels(t *testing.T) {
	m := newNoteMatrixModel(0)
	if got := m.RowLabel(0); got != "B" {
		t.Errorf("top row label = %q, want B", got)
	}
	if got := m.RowLabel(noteMatrixRows - 1); got != "C" {
		t.Errorf("bottom row label = %q, want C", got)
	}
	// Column labels are octave numbers and must ascend.
	if m.ColLabel(0) >= m.ColLabel(m.Cols()-1) {
		t.Errorf("column labels %q..%q do not ascend", m.ColLabel(0), m.ColLabel(m.Cols()-1))
	}
	// Every row label must be a real note name.
	for row := 0; row < noteMatrixRows; row++ {
		if m.RowLabel(row) == "" {
			t.Errorf("row %d has no label", row)
		}
	}
}

func TestNoteMatrix_BlackKeys(t *testing.T) {
	m := newNoteMatrixModel(0)
	black := 0
	for row := 0; row < noteMatrixRows; row++ {
		isBlack := m.isBlackKey(row)
		name := m.RowLabel(row)
		wantBlack := len(name) == 2 && name[1] == '#'
		if isBlack != wantBlack {
			t.Errorf("row %d (%s): isBlackKey=%v, want %v", row, name, isBlack, wantBlack)
		}
		if isBlack {
			black++
		}
	}
	if black != 5 {
		t.Errorf("got %d black keys, want 5", black)
	}
}

// The matrix label must agree with nodeNoteLabel, the shared formatter the
// sidebar chip and coord badge use. Two formatters that disagree would let a
// user pick "C4" and see "B3".
func TestNoteMatrix_AgreesWithNodeNoteLabel(t *testing.T) {
	g := newTestGameForUndo(t)
	node := noteTestNode(t, g)
	g.drum.Rows[0].Instrument = roster.OrganChurch

	offset, pitched := audio.InstrumentPitchInfo(roster.OrganChurch)
	if !pitched {
		t.Skip("organ-church is not pitched in this build")
	}
	m := newNoteMatrixModel(offset)

	for _, pitch := range []float64{-24, -12, -5, 0, 7, 12, 24} {
		col, row, ok := m.CellFor(pitch)
		if !ok {
			t.Errorf("pitch %v has no cell", pitch)
			continue
		}
		want := g.nodeNoteLabel(node.ID, pitch)
		if got := m.NoteLabelAt(col, row); got != want {
			t.Errorf("pitch %v: matrix says %q, nodeNoteLabel says %q", pitch, got, want)
		}
	}
}
