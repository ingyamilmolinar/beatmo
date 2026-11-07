package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The fast path to a specific note; −/+ still nudge. The same SHAPE as the colour picker so it shares
// solveSwatchGrid and the portal wiring; the cell↔pitch mapping lives in noteMatrixModel.

// NoteMatrixProps is the external state, mirroring ColorWheelProps.
type NoteMatrixProps struct {
	// AnchorRect is the note chip that opens the picker (used for positioning).
	AnchorRect image.Rectangle
	// Bounds is the container the panel is clamped inside.
	Bounds image.Rectangle
	// CurrentPitch is the node's semitone pitch; its cell renders selected.
	CurrentPitch float64
	// So the labels match what the node sounds.
	InstrumentOffset float64
	// OnPick is called with the node pitch for the tapped cell.
	OnPick func(pitch float64)
	// OnClose is called when the picker should close.
	OnClose func()
}

// Rows are the 12 pitch classes and columns octaves: tall and narrow keeps every cell at the touch minimum on a phone.
const (
	noteMatrixGap      = gridPickerGap
	noteMatrixCellMin  = 34 // desktop minimum cell edge
	noteMatrixGutter   = 30 // left gutter for the pitch-class labels
	noteMatrixDeskMaxW = 320
)

type NoteMatrixComponent struct {
	*GridPickerComponent
	props NoteMatrixProps
	model noteMatrixModel
}

// NewNoteMatrixComponent creates an empty note-matrix picker.
func NewNoteMatrixComponent() *NoteMatrixComponent {
	return &NoteMatrixComponent{GridPickerComponent: newGridPickerComponent(gridPickerSpec{
		Gutter: noteMatrixGutter, CellMin: noteMatrixCellMin, DeskMaxW: noteMatrixDeskMaxW,
	})}
}

// SetProps updates the external props.
func (c *NoteMatrixComponent) SetProps(p NoteMatrixProps) {
	c.props = p
	c.setFrame(p.AnchorRect, p.Bounds)
}

// Props returns the current props.
func (c *NoteMatrixComponent) Props() NoteMatrixProps { return c.props }

// Open opens the picker and rebuilds its layout.
func (c *NoteMatrixComponent) Open() {
	c.model = newNoteMatrixModel(c.props.InstrumentOffset)
	c.openWith(noteGridModel{c}, c.props.OnClose)
}

// Model exposes the cell↔pitch mapping for tests.
func (c *NoteMatrixComponent) Model() noteMatrixModel { return c.model }

// The sidebar re-syncs the live pitch every frame.
type noteGridModel struct{ c *NoteMatrixComponent }

func (m noteGridModel) Cols() int                     { return m.c.model.Cols() }
func (m noteGridModel) Rows() int                     { return noteMatrixRows }
func (m noteGridModel) RowLabel(row int) string       { return m.c.model.RowLabel(row) }
func (m noteGridModel) CellLabel(col, row int) string { return m.c.model.NoteLabelAt(col, row) }
func (m noteGridModel) CellSelectable(col, row int) bool {
	_, ok := m.c.model.PitchAt(col, row)
	return ok
}
func (m noteGridModel) CellShaded(row int) bool { return m.c.model.isBlackKey(row) }
func (m noteGridModel) Selected() (int, int, bool) {
	return m.c.model.CellFor(m.c.props.CurrentPitch)
}

// States what it is about to change.
func (m noteGridModel) Title() string {
	if col, row, ok := m.Selected(); ok {
		return m.c.model.NoteLabelAt(col, row)
	}
	return i18n.T(i18n.KeyCapPitch)
}

// The sidebar owns the buttons, so cells get the same hover, press and deferred-tap behaviour as every node-menu control.
type noteMatrixCell struct {
	ID         sbID // the cell's sidebar button
	Rect       image.Rectangle
	Label      string  // "C#4"
	Pitch      float64 // node pitch this cell selects
	Selectable bool    // false for cells outside the pitch range
	Selected   bool    // the node's current note
	Black      bool    // accidental, toned like a piano's black key
}

// VisibleCells enumerates the cells currently inside the scroll window.
func (c *NoteMatrixComponent) VisibleCells() []noteMatrixCell {
	cells := c.visibleCells()
	if cells == nil {
		return nil
	}
	out := make([]noteMatrixCell, 0, len(cells))
	for _, g := range cells {
		pitch, _ := c.model.PitchAt(g.Col, g.Row)
		out = append(out, noteMatrixCell{
			ID: sbNoteCell(g.Col, g.Row), Rect: g.Rect, Label: g.Label, Pitch: pitch,
			Selectable: g.Selectable, Selected: g.Selected, Black: g.Shaded,
		})
	}
	return out
}
