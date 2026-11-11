package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// The chord matrix model: 12 root rows (C…B) × the five qualities of a page.
func TestChordMatrixModelShape(t *testing.T) {
	m := chordMatrixModel{page: 1, rootPitch: 3, spec: model.ChordSpec{Quality: "m7"}} // A3+3 = C4
	if m.Rows() != 12 || m.Cols() != 5 {
		t.Fatalf("shape %dx%d, want 12 rows × 5 cols", m.Rows(), m.Cols())
	}
	if got := m.RowLabel(1); got != "C#" {
		t.Fatalf("row 1 label %q, want C#", got)
	}
	if got := m.CellLabel(2, 1); got != "C#m7" {
		t.Fatalf("7ths page col 2 row C# = %q, want C#m7", got)
	}
	col, row, ok := m.Selected()
	if !ok || col != 2 || row != 0 {
		t.Fatalf("selected = (%d,%d,%v), want m7 on C = (2,0)", col, row, ok)
	}
	if !m.CellShaded(1) || m.CellShaded(0) {
		t.Fatal("accidental rows shade like black keys")
	}
	m.page = 0
	if _, _, ok := m.Selected(); ok {
		t.Fatal("m7 is not on the Triads page: nothing is selected there")
	}
}

// Picking keeps the root in the CURRENT root's octave and sets the quality;
// the inversion is re-clamped to the new quality, the spread kept.
func TestChordMatrixPickKeepsTheRootOctave(t *testing.T) {
	// Root C4 (pitch 3); pick Triads col 1 (m) on row 7 (G) → G4 = pitch 10.
	m := chordMatrixModel{page: 0, rootPitch: 3, spec: model.ChordSpec{Quality: "maj7", Inversion: 3, Spread: 2}}
	pitch, spec := m.PickAt(1, 7)
	if pitch != 10 {
		t.Fatalf("G picked from C4 → pitch %v, want 10 (G4)", pitch)
	}
	if spec.Quality != "m" || spec.Inversion != 2 || spec.Spread != 2 {
		t.Fatalf("spec %+v, want m / inversion clamped to 2 / spread kept", spec)
	}
	pitch, _ = m.PickAt(0, 0) // C again
	if pitch != 3 {
		t.Fatalf("C from C4 → %v, want 3", pitch)
	}
}

// Review Focus 4: near the ±24 edge the picked root folds by an octave rather
// than leaving the legal pitch range.
func TestChordMatrixPickClampsToThePitchRange(t *testing.T) {
	hi := chordMatrixModel{rootPitch: model.NodePitchMax - 1, spec: model.ChordSpec{Quality: "maj"}} // G#5
	for row := 0; row < 12; row++ {
		p, _ := hi.PickAt(0, row)
		if p < model.NodePitchMin || p > model.NodePitchMax {
			t.Fatalf("row %d from the top edge → pitch %v outside ±24", row, p)
		}
	}
	lo := chordMatrixModel{rootPitch: model.NodePitchMin, spec: model.ChordSpec{Quality: "maj"}}
	for row := 0; row < 12; row++ {
		p, _ := lo.PickAt(0, row)
		if p < model.NodePitchMin || p > model.NodePitchMax {
			t.Fatalf("row %d from the bottom edge → pitch %v outside ±24", row, p)
		}
	}
}

// The cells name chords on the same MIDI root the Pitch chip names (the
// instrument's live offset included).
func TestChordMatrixHonoursTheInstrumentOffset(t *testing.T) {
	m := chordMatrixModel{rootPitch: 0, instOffset: 3, spec: model.ChordSpec{Quality: "maj"}} // A3 + 3 = C4
	if _, row, ok := m.Selected(); !ok || row != 0 {
		t.Fatalf("root should read C with a +3 offset, row=%d ok=%v", row, ok)
	}
}
