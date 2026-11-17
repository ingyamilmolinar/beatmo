package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// NoteMatrixComponent is the note/octave picker opened from the note chip in
// the node sidebar's Pitch section. It is the fast path to a specific note;
// the −/+ buttons still work for nudging by a semitone.
//
// It is deliberately the same SHAPE as the colour picker
// (ColorWheelComponent): an anchored panel, a header, a left gutter of row
// labels and a fixed cell grid. Both solve their geometry with
// solveSwatchGrid, and this one satisfies the same compOverlayable interface,
// so it inherits the portal wiring (modal filtering, tap-away-to-close,
// capture) without adding any input or drawing primitives.
//
// The mapping between cells and node pitch lives in noteMatrixModel; this file
// is layout, input and drawing only.

// NoteMatrixProps is the external state, mirroring ColorWheelProps.
type NoteMatrixProps struct {
	// AnchorRect is the note chip that opens the picker (used for positioning).
	AnchorRect image.Rectangle
	// Bounds is the container the panel is clamped inside.
	Bounds image.Rectangle
	// CurrentPitch is the node's semitone pitch; its cell renders selected.
	CurrentPitch float64
	// InstrumentOffset is the row instrument's live pitch offset, so the cell
	// labels match what the node will actually sound.
	InstrumentOffset float64
	// OnPick is called with the node pitch for the tapped cell.
	OnPick func(pitch float64)
	// OnClose is called when the picker should close.
	OnClose func()
}

// Grid tuning. Rows are the 12 pitch classes; columns are octaves, so the
// panel is tall and narrow like the colour picker rather than 12 cells wide —
// which is what keeps every cell at the touch minimum on a phone.
const (
	noteMatrixGap      = gridPickerGap
	noteMatrixCellMin  = 34 // desktop minimum cell edge
	noteMatrixGutter   = 30 // left gutter for the pitch-class labels
	noteMatrixDeskMaxW = 320
)

// NoteMatrixComponent implements compOverlayable through the shared
// GridPickerComponent (geometry, row scrolling, chrome, panel input); this
// type only supplies the note model.
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

// noteGridModel adapts the note model (and the LIVE current pitch, which the
// sidebar re-syncs every frame) to gridPickerModel.
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

// Title names the note the node currently sounds, so the picker states what
// it is about to change rather than leaving the user to infer it.
func (m noteGridModel) Title() string {
	if col, row, ok := m.Selected(); ok {
		return m.c.model.NoteLabelAt(col, row)
	}
	return i18n.T(i18n.KeyCapPitch)
}

// noteMatrixCell is one grid cell, published for the sidebar to render and
// wire as a real Button. The component owns geometry; the sidebar owns the
// buttons so cells get the same hover, press animation and deferred-tap
// behaviour as every other node-menu control.
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
