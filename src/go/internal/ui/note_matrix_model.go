package ui

import (
	"math"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// noteMatrixModel is the pure mapping between a node's semitone pitch and a
// cell in the note-matrix picker. It holds no rects and no UI state, so the
// mapping is testable on its own and the component below stays layout-only.
//
// Shape: 12 pitch-class ROWS × one COLUMN per octave — the same shape as the
// colour picker (a handful of columns, many rows, row labels in a left
// gutter), which is what lets both share solveSwatchGrid AND what makes every
// cell clear the 44 px touch minimum on a phone. A 12-column layout could not:
// 12 × 44 = 528 px against a 390 px portrait screen.
//
// Rows run HIGH AT TOP (row 0 = B, row 11 = C) and columns ascend left to
// right, so pitch increases upward and rightward — matching the "larger
// numbers on top" convention the pitch wheel already established.
type noteMatrixModel struct {
	// offset is the row instrument's live pitch offset in semitones. Node
	// pitch 0 sounds A3 (MIDI 57) plus this.
	offset float64
	// loOct is the octave number of the leftmost column.
	loOct int
	// cols is the number of octave columns needed to cover the pitch range.
	cols int
}

// noteMatrixRows is fixed: the 12 pitch classes.
const noteMatrixRows = 12

// a3MIDI is the MIDI note a node with pitch 0 sounds before the instrument's
// own offset — the engine's A3 = pitch-0 convention.
const a3MIDI = 57

// newNoteMatrixModel builds the model for an instrument offset, covering
// exactly model.NodePitchMin..NodePitchMax.
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

// midiForPitch converts a node pitch to the MIDI note it sounds. Delegates to
// nodePitchMIDI so the picker and the note chip cannot drift apart.
func (m noteMatrixModel) midiForPitch(pitch float64) int {
	return nodePitchMIDI(pitch, m.offset)
}

// midiOctave is the octave number printed alongside a note name, matching
// midiNoteName's convention (MIDI 60 = C4). Floor division keeps numbering
// monotonic below MIDI 0, which the deep-kick offsets can now reach.
func midiOctave(midi int) int { return int(math.Floor(float64(midi)/12)) - 1 }

// Cols reports the number of octave columns.
func (m noteMatrixModel) Cols() int { return m.cols }

// MIDIAt returns the MIDI note of a cell. Row 0 is the TOP row and holds B,
// so higher rows sound lower.
func (m noteMatrixModel) MIDIAt(col, row int) int {
	pc := noteMatrixRows - 1 - row // row 0 -> 11 (B), row 11 -> 0 (C)
	return (m.loOct+col+1)*12 + pc
}

// PitchAt returns the node pitch a cell selects. ok is false when the cell
// falls outside the selectable range — those cells render dimmed and are not
// hit-testable, since the grid is a rectangle while the range is a run.
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

// CellFor returns the cell holding a node pitch. ok is false when the pitch
// lies outside the matrix (a node edited past the range by other means still
// shows no selection rather than a wrong one).
func (m noteMatrixModel) CellFor(pitch float64) (col, row int, ok bool) {
	// Range check first: the grid is a RECTANGLE covering more notes than the
	// selectable run, so an out-of-range pitch still lands on a valid (col,row)
	// and would otherwise light up a cell that cannot produce it.
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

// NoteLabelAt is the full name of a cell ("C#4"), used for the selected-cell
// readout and by tests.
func (m noteMatrixModel) NoteLabelAt(col, row int) string {
	return midiNoteName(m.MIDIAt(col, row))
}

// isBlackKey reports whether a row is an accidental, so the grid can tone it
// like a piano's black keys.
func (m noteMatrixModel) isBlackKey(row int) bool {
	switch noteMatrixRows - 1 - row {
	case 1, 3, 6, 8, 10: // C#, D#, F#, G#, A#
		return true
	}
	return false
}
