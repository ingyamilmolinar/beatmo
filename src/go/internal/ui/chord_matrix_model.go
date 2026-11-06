package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// chordMatrixModel is the chord matrix's cell mapping
// (docs/superpowers/specs/2026-10-09-chord-nodes-design.md §3): 12 root ROWS
// (pitch classes C…B, top to bottom) × the five quality COLUMNS of one page of
// model.ChordQualities. Five columns is what fits a phone at the touch
// minimum; the pages (Triads · 7ths · Ext · Sus) carry the rest.
type chordMatrixModel struct {
	page       int             // index into model.ChordPages
	rootPitch  float64         // the node's Pitch (the chord root)
	instOffset float64         // the row instrument's live pitch offset
	spec       model.ChordSpec // the node's current chord
}

func (m chordMatrixModel) qualities() []model.ChordQuality {
	if m.page < 0 || m.page >= len(model.ChordPages) {
		return nil
	}
	return model.ChordQualitiesOnPage(model.ChordPages[m.page])
}

func (m chordMatrixModel) rootMIDI() int { return nodePitchMIDI(m.rootPitch, m.instOffset) }

func (m chordMatrixModel) Cols() int               { return len(m.qualities()) }
func (m chordMatrixModel) Rows() int               { return 12 }
func (m chordMatrixModel) RowLabel(row int) string { return model.PitchClassName(row) }

// CellLabel is the chord's name in root position ("C#m7").
func (m chordMatrixModel) CellLabel(col, row int) string {
	qs := m.qualities()
	if col < 0 || col >= len(qs) {
		return ""
	}
	return model.ChordName(row, model.ChordSpec{Quality: qs[col].ID})
}

// CellSelectable: every cell is reachable — PickAt folds the root by an
// octave rather than leaving the legal pitch range.
func (m chordMatrixModel) CellSelectable(col, row int) bool {
	return col >= 0 && col < m.Cols() && row >= 0 && row < 12
}

// CellShaded tones accidental roots like a piano's black keys.
func (m chordMatrixModel) CellShaded(row int) bool {
	switch row {
	case 1, 3, 6, 8, 10:
		return true
	}
	return false
}

// Selected is the node's current chord, when its quality is on this page.
func (m chordMatrixModel) Selected() (col, row int, ok bool) {
	q := m.spec.Quality
	if q == "" {
		q = model.ChordDefaultQuality
	}
	for i, cq := range m.qualities() {
		if cq.ID == q {
			return i, ((m.rootMIDI() % 12) + 12) % 12, true
		}
	}
	return 0, 0, false
}

// Title names the chord the node currently sounds.
func (m chordMatrixModel) Title() string {
	if m.spec.IsZero() {
		return i18n.T(i18n.KeyChordMatrixTitle)
	}
	return model.ChordName(m.rootMIDI(), m.spec)
}

// PickAt is what picking (col, row) does: the root moves to that pitch class
// IN THE CURRENT ROOT'S OCTAVE (folded by an octave to stay inside
// NodePitchMin/Max), the quality becomes the column's, the inversion is
// re-clamped to it and the spread is kept.
func (m chordMatrixModel) PickAt(col, row int) (pitch float64, spec model.ChordSpec) {
	qs := m.qualities()
	if col < 0 || col >= len(qs) {
		return m.rootPitch, m.spec
	}
	r := m.rootMIDI()
	base := r - ((r%12)+12)%12
	pitch = m.rootPitch + float64(base+row-r)
	for pitch > model.NodePitchMax {
		pitch -= 12
	}
	for pitch < model.NodePitchMin {
		pitch += 12
	}
	spec = m.spec
	spec.Quality = qs[col].ID
	return pitch, model.NormalizeChord(spec)
}
