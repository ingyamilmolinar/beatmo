package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The second instance of GridPickerComponent; this type supplies the chord model and the page bar.
type ChordMatrixComponent struct {
	*GridPickerComponent
	m chordMatrixModel

	// The host re-seats its keyboard cursor so it never sits on a page that does not hold the current chord.
	OnPageChange func()
}

const (
	chordMatrixCellMin  = 40 // chord names are wider than note names
	chordMatrixGutter   = 30
	chordMatrixDeskMaxW = 360
	chordMatrixPageBarH = 32
)

// NewChordMatrixComponent creates an empty chord picker.
func NewChordMatrixComponent() *ChordMatrixComponent {
	return &ChordMatrixComponent{GridPickerComponent: newGridPickerComponent(gridPickerSpec{
		Gutter: chordMatrixGutter, CellMin: chordMatrixCellMin, DeskMaxW: chordMatrixDeskMaxW,
		BandH: chordMatrixPageBarH,
	})}
}

func chordPageLabels() []string {
	return []string{
		i18n.T(i18n.KeyChordPageTriads), i18n.T(i18n.KeyChordPageSevenths),
		i18n.T(i18n.KeyChordPageExt), i18n.T(i18n.KeyChordPageSus),
	}
}

func (c *ChordMatrixComponent) Open(anchor, container image.Rectangle, rootPitch, instOffset float64, spec model.ChordSpec, onClose func()) {
	q := spec.Quality
	if q == "" {
		q = model.ChordDefaultQuality
	}
	c.m = chordMatrixModel{page: model.ChordPageOf(q), rootPitch: rootPitch, instOffset: instOffset, spec: spec}
	c.setFrame(anchor, container)
	c.setPages(chordPageLabels(), c.m.page, c.SetPage)
	c.openWith(chordGridModel{c}, onClose)
}

// The sidebar re-lays out every frame, so the anchor moves.
func (c *ChordMatrixComponent) Sync(anchor image.Rectangle, rootPitch float64, spec model.ChordSpec) {
	c.anchor = anchor
	c.m.rootPitch, c.m.spec = rootPitch, spec
}

// Page is the visible quality page.
func (c *ChordMatrixComponent) Page() int { return c.m.page }

// Every page has the same shape, so the geometry does not change.
func (c *ChordMatrixComponent) SetPage(i int) {
	n := len(model.ChordPages)
	c.m.page = ((i % n) + n) % n
	if c.pages != nil {
		c.pages.SetActive(c.m.page)
	}
	if c.OnPageChange != nil {
		c.OnPageChange()
	}
}

// Model exposes the cell mapping.
func (c *ChordMatrixComponent) Model() chordMatrixModel { return c.m }

// chordGridModel reads the component's LIVE model (page, root, spec).
type chordGridModel struct{ c *ChordMatrixComponent }

func (m chordGridModel) Cols() int                        { return m.c.m.Cols() }
func (m chordGridModel) Rows() int                        { return m.c.m.Rows() }
func (m chordGridModel) RowLabel(row int) string          { return m.c.m.RowLabel(row) }
func (m chordGridModel) CellLabel(col, row int) string    { return m.c.m.CellLabel(col, row) }
func (m chordGridModel) CellSelectable(col, row int) bool { return m.c.m.CellSelectable(col, row) }
func (m chordGridModel) CellShaded(row int) bool          { return m.c.m.CellShaded(row) }
func (m chordGridModel) Selected() (int, int, bool)       { return m.c.m.Selected() }
func (m chordGridModel) Title() string                    { return m.c.m.Title() }
