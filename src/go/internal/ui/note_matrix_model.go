package ui

import (
	"math"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Pure (no rects, no UI state). 12 rows × an octave per column: a 12-column grid could not keep 44 px cells on
// a 390 px screen. High notes at TOP and right, matching the pitch wheel.
type noteMatrixModel struct {
	// Node pitch 0 sounds A3 (MIDI 57) plus this.
	offset float64
	// loOct is the octave number of the leftmost column.
	loOct int
	// cols is the number of octave columns needed to cover the pitch range.
	cols int
}

// noteMatrixRows is fixed: the 12 pitch classes.
const noteMatrixRows = 12

// The engine's A3 = pitch-0 convention.
const a3MIDI = 57

// Exactly NodePitchMin..NodePitchMax.
func newNoteMatrixModel(offset float64) noteMatrixModel {
	m := noteMatrixModel{offset: offset}
	loMIDI := m.midiForPitch(model.NodePitchMin)
	hiMIDI := m.midiForPitch(model.NodePitchMax)
	m.loOct = midiOctave(loMIDI)
	m.cols = midiOctave(hiMIDI) - m.loOct + 1
	if m.cols < 1 {
		m.cols = 1
	}
	return m
}

// Through nodePitchMIDI, so the picker and the note chip cannot drift apart.
func (m noteMatrixModel) midiForPitch(pitch float64) int {
	return nodePitchMIDI(pitch, m.offset)
}

// MIDI 60 = C4, as midiNoteName; floor division stays monotonic below MIDI 0, which deep kicks reach.
func midiOctave(midi int) int { return int(math.Floor(float64(midi)/12)) - 1 }

// Cols reports the number of octave columns.
func (m noteMatrixModel) Cols() int { return m.cols }

// Row 0 is the TOP row and holds B.
func (m noteMatrixModel) MIDIAt(col, row int) int {
	pc := noteMatrixRows - 1 - row // row 0 -> 11 (B), row 11 -> 0 (C)
	return (m.loOct+col+1)*12 + pc
}

// !ok outside the selectable run: the grid is a rectangle, the range a run, so those cells are dimmed and inert.
func (m noteMatrixModel) PitchAt(col, row int) (float64, bool) {
	if col < 0 || col >= m.cols || row < 0 || row >= noteMatrixRows {
		return 0, false
	}
	pitch := float64(m.MIDIAt(col, row)) - m.offset - float64(a3MIDI)
	if pitch < model.NodePitchMin || pitch > model.NodePitchMax {
		return 0, false
	}
	return pitch, true
}

// !ok outside the matrix: no selection rather than a wrong one.
func (m noteMatrixModel) CellFor(pitch float64) (col, row int, ok bool) {
	// Range first: an out-of-range pitch still lands on a valid cell that cannot produce it.
	if pitch < model.NodePitchMin || pitch > model.NodePitchMax {
		return 0, 0, false
	}
	midi := m.midiForPitch(pitch)
	col = midiOctave(midi) - m.loOct
	row = noteMatrixRows - 1 - (midi % 12)
	if col < 0 || col >= m.cols || row < 0 || row >= noteMatrixRows {
		return 0, 0, false
	}
	return col, row, true
}

// RowLabel is the pitch-class name for a row ("C", "C#", …), for the gutter.
func (m noteMatrixModel) RowLabel(row int) string {
	pc := noteMatrixRows - 1 - row
	if pc < 0 || pc >= len(noteNames) {
		return ""
	}
	return noteNames[pc]
}

// ColLabel is the octave number for a column, drawn as a column header.
func (m noteMatrixModel) ColLabel(col int) string {
	return strconv.Itoa(m.loOct + col)
}

func (m noteMatrixModel) NoteLabelAt(col, row int) string {
	return midiNoteName(m.MIDIAt(col, row))
}

// Toned like a piano's black keys.
func (m noteMatrixModel) isBlackKey(row int) bool {
	switch noteMatrixRows - 1 - row {
	case 1, 3, 6, 8, 10: // C#, D#, F#, G#, A#
		return true
	}
	return false
}
