package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// GridPickerComponent is the shared machinery behind the node sidebar's grid
// pickers — the note matrix (NoteMatrixComponent) and the chord matrix
// (ChordMatrixComponent). It owns an anchored panel solved by solveSwatchGrid
// (the colour picker's geometry), a header with the title and close button,
// a left gutter of row labels, an optional page bar (SegmentedControl) and a
// ROW-scrolling cell grid that keeps every cell at the touch minimum.
//
// What a cell MEANS lives in a gridPickerModel; the hosting sidebar turns the
// visible cells into real Buttons (so they share hover, press animation and
// deferred-tap dispatch with every other node-menu control) and maps a cell
// to its action.
type GridPickerComponent struct {
	overlayBase

	anchor    image.Rectangle
	container image.Rectangle
	onClose   func()
	spec      gridPickerSpec

	open  bool
	model gridPickerModel
	lay   swatchGridLayout

	closeBtn *Button
	pages    *SegmentedControl // nil: no page bar

	// scroll pages the ROWS when the container is too short to show them all
	// at a touch-sized height. Columns always stay fully visible.
	scroll  *ScrollBehavior
	visRows int
	rowH    int

	// Gesture state for the press → drag-or-pick resolution in HandleInput.
	pressing bool
	dragging bool
	pressY   int
}

// gridPickerModel is what a grid picker shows. Rows scroll; columns do not.
type gridPickerModel interface {
	Cols() int
	Rows() int
	RowLabel(row int) string
	CellLabel(col, row int) string
	CellSelectable(col, row int) bool
	// CellShaded tones a row like a piano's black key.
	CellShaded(row int) bool
	// Selected is the cell holding the current value.
	Selected() (col, row int, ok bool)
	Title() string
}

// gridPickerSpec is a picker's fixed geometry.
type gridPickerSpec struct {
	Gutter   int // left label gutter
	CellMin  int // desktop minimum cell edge
	DeskMaxW int // desktop panel width cap
	BandH    int // page-bar band above the cells (0 = none)
}

const (
	gridPickerGap    = 4
	gridPickerPad    = 10
	gridPickerHeader = 32
	// gridPickerDragSlopPx is how far a press may travel before it counts as
	// a scroll rather than a pick.
	gridPickerDragSlopPx = 6
)

// gridPickerCell is one visible cell, published for the host to wire as a
// Button.
type gridPickerCell struct {
	Col, Row   int
	Rect       image.Rectangle
	Label      string
	Selectable bool
	Selected   bool
	Shaded     bool
}

func newGridPickerComponent(spec gridPickerSpec) *GridPickerComponent {
	return &GridPickerComponent{
		overlayBase: newOverlayBase(),
		spec:        spec,
		scroll:      NewScrollBehavior(ScrollbarStyleForPlatform(), 1),
	}
}

// setFrame sets where the picker anchors and what it is clamped inside.
func (c *GridPickerComponent) setFrame(anchor, container image.Rectangle) {
	c.anchor, c.container = anchor, container
}

// setPages installs (or, with nil labels, removes) the page bar.
func (c *GridPickerComponent) setPages(labels []string, active int, onPick func(int)) {
	if len(labels) == 0 {
		c.pages = nil
		return
	}
	c.pages = NewSegmentedControl(labels, active, onPick)
}

// open shows the picker over model.
func (c *GridPickerComponent) openWith(model gridPickerModel, onClose func()) {
	c.model, c.onClose = model, onClose
	c.open = true
	c.pressing, c.dragging = false, false
	c.rebuild()
	c.revealSelected()
}

// Close closes the picker.
func (c *GridPickerComponent) Close() {
	if !c.open {
		return
	}
	c.open = false
	c.pressing, c.dragging = false, false
	if c.onClose != nil {
		c.onClose()
	}
}

// IsOpen reports whether the picker is open.
func (c *GridPickerComponent) IsOpen() bool { return c.open }

// Layout exposes the solved geometry for tests.
func (c *GridPickerComponent) Layout() swatchGridLayout { return c.lay }

// CloseBtn exposes the close button (nil on mobile).
func (c *GridPickerComponent) CloseBtn() *Button { return c.closeBtn }

// Pages exposes the page bar (nil when the picker has none).
func (c *GridPickerComponent) Pages() *SegmentedControl { return c.pages }

// rebuild recomputes the panel geometry and the row window.
func (c *GridPickerComponent) rebuild() {
	if c.model == nil {
		return
	}
	rows := c.model.Rows()
	c.lay = solveSwatchGrid(swatchGridSpec{
		Anchor:      c.anchor,
		Bounds:      c.container,
		Cols:        c.model.Cols(),
		Rows:        rows,
		Gutter:      c.spec.Gutter,
		Gap:         gridPickerGap,
		Pad:         gridPickerPad,
		Header:      gridPickerHeader,
		CellMin:     c.spec.CellMin,
		DesktopMaxW: c.spec.DeskMaxW,
		BandH:       c.spec.BandH,
	})
	c.SetBounds(c.lay.Panel)
	if c.pages != nil {
		band := c.lay.Band
		if band.Empty() && c.spec.BandH > 0 {
			// solveSwatchGrid drops the band when the container is short (the
			// grid pane often is). The page bar is not optional chrome — it is
			// how three of the four pages are reached — so carve it out of the
			// top of the cell region instead; the rows scroll in what is left.
			band = image.Rect(c.lay.Cells.Min.X, c.lay.Cells.Min.Y, c.lay.Cells.Max.X, c.lay.Cells.Min.Y+c.spec.BandH)
			c.lay.Cells.Min.Y = band.Max.Y
		}
		c.pages.SetRect(image.Rect(band.Min.X, band.Min.Y, band.Max.X, band.Max.Y-gridPickerGap))
	}

	// Row window. A cell must stay at least touch-min tall; when the solved
	// cell region cannot hold every row at that height, show as many as fit
	// and scroll the rest.
	minRowH := c.spec.CellMin
	if tm := TouchMinTarget(); Profile().IsMobile() && tm > minRowH {
		minRowH = tm
	}
	c.rowH = c.lay.CellH + gridPickerGap
	if c.lay.CellH < minRowH {
		c.lay.CellH = minRowH
		c.rowH = minRowH + gridPickerGap
	}
	c.visRows = rows
	if c.rowH > 0 {
		if v := (c.lay.Cells.Dy() + gridPickerGap) / c.rowH; v < rows {
			c.visRows = v
		}
	}
	if c.visRows < 1 {
		c.visRows = 1
	}
	c.scroll.VS.Total = rows
	c.scroll.VS.Visible = c.visRows
	c.scroll.VS.View = c.lay.Cells
	c.scroll.VS.Clamp()

	c.closeBtn = newPopupCloseButton(func() { c.Close() })
	if c.closeBtn != nil {
		c.closeBtn.SetRect(closeButtonRect(c.lay.Title, SpaceXS))
	}
}

// revealSelected keeps the current value in view, so opening the picker
// never hides what the user is about to change.
func (c *GridPickerComponent) revealSelected() {
	if c.model == nil {
		return
	}
	if _, row, ok := c.model.Selected(); ok {
		c.RevealRow(row)
	}
}

// visibleCells enumerates the cells currently inside the scroll window.
func (c *GridPickerComponent) visibleCells() []gridPickerCell {
	if !c.open || c.model == nil {
		return nil
	}
	selCol, selRow, hasSel := c.model.Selected()
	out := make([]gridPickerCell, 0, c.model.Cols()*c.visRows)
	for col := 0; col < c.model.Cols(); col++ {
		for row := 0; row < c.model.Rows(); row++ {
			r, visible := c.cellRect(col, row)
			if !visible {
				continue
			}
			out = append(out, gridPickerCell{
				Col: col, Row: row, Rect: r,
				Label:      c.model.CellLabel(col, row),
				Selectable: c.model.CellSelectable(col, row),
				Selected:   hasSel && col == selCol && row == selRow,
				Shaded:     c.model.CellShaded(row),
			})
		}
	}
	return out
}

// cellRect is the on-screen rect of a MODEL row, or ok=false when that row is
// scrolled out of the window.
func (c *GridPickerComponent) cellRect(col, row int) (image.Rectangle, bool) {
	vrow := row - c.scroll.VS.First
	if vrow < 0 || vrow >= c.visRows {
		return image.Rectangle{}, false
	}
	return c.lay.CellRect(col, vrow, gridPickerGap), true
}

// RevealRow scrolls the row window so row is visible. Used by the keyboard
// cursor, whose index counts CELLS while this scroller counts ROWS.
func (c *GridPickerComponent) RevealRow(row int) {
	if c.visRows <= 0 {
		return
	}
	switch {
	case row < c.scroll.VS.First:
		c.scroll.VS.First = row
	case row >= c.scroll.VS.First+c.visRows:
		c.scroll.VS.First = row - c.visRows + 1
	default:
		return
	}
	c.scroll.VS.Clamp()
}

// CellsRect is the region the cells tile.
func (c *GridPickerComponent) CellsRect() image.Rectangle { return c.lay.Cells }

// HasScroll reports whether the rows overflow the window.
func (c *GridPickerComponent) HasScroll() bool { return c.scroll.HasScroll() }

// InputBounds is the panel rect.
func (c *GridPickerComponent) InputBounds() image.Rectangle { return c.bounds }

// Capturing reports whether the component holds pointer capture.
func (c *GridPickerComponent) Capturing() bool { return c.open && c.pressing }

// HandleInput handles only the PANEL-level gestures: the close button, the
// page bar, a drag that scrolls the rows, and a press outside that dismisses
// the picker.
//
// Cells are NOT handled here. They are real Buttons owned by the sidebar, so
// they inherit hover styling, the press animation and the deferred-tap
// resolution every other node-menu control uses. An earlier version
// hit-tested cells inside the component and was completely DEAD in the app:
// the sidebar resolves taps through buttonOrder(), so a control that is not a
// registered Button never fires.
func (c *GridPickerComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !c.open {
		return InputIgnored
	}
	if c.closeBtn != nil && c.closeBtn.HandleInputResult(x, y, pressed) != InputIgnored {
		return InputConsumed
	}

	pt := image.Pt(x, y)
	if !pressed {
		if c.pressing {
			c.pressing, c.dragging = false, false
			return InputConsumed
		}
		if pt.In(c.bounds) {
			return InputConsumed
		}
		return InputIgnored
	}

	// Held: a drag inside the cells scrolls the row window.
	if c.pressing {
		if !c.dragging && abs(y-c.pressY) > gridPickerDragSlopPx {
			c.dragging = true
		}
		if c.dragging && c.rowH > 0 {
			if rows := (c.pressY - y) / c.rowH; rows != 0 {
				c.scroll.VS.ScrollBy(rows)
				c.pressY -= rows * c.rowH
			}
		}
		return InputCaptured
	}

	if pt.In(c.bounds) {
		// The page bar switches pages on the press edge (like every other
		// SegmentedControl).
		if c.pages != nil && c.pages.HitTest(x, y) {
			return InputConsumed
		}
		// Only arm a drag over the cells; chrome presses are absorbed so they
		// neither pick nor dismiss.
		if pt.In(c.lay.Cells) && c.scroll.HasScroll() {
			c.pressing = true
			c.pressY = y
			c.dragging = false
		}
		return InputConsumed
	}

	// Pressed outside the panel: dismiss.
	c.Close()
	return InputConsumed
}

// HandleWheel scrolls the rows when the pointer is over the panel.
func (c *GridPickerComponent) HandleWheel(x, y, steps int) InputResult {
	if !c.open || !image.Pt(x, y).In(c.bounds) {
		return InputIgnored
	}
	c.scroll.HandleWheel(steps)
	return InputConsumed
}

// DrawChrome paints everything EXCEPT the cells: panel, header, page bar and
// gutter labels. The sidebar draws the cell Buttons between the chrome and
// DrawOverlay so they get the shared keycap look.
func (c *GridPickerComponent) DrawChrome(dst *ebiten.Image) {
	if !c.open || c.bounds.Empty() || c.model == nil {
		return
	}
	// Shared menu chrome, same as the colour picker.
	drawPanel(dst, c.bounds)
	drawMenuHeaderBand(dst, c.lay.Title)
	drawMenuTitle(dst, image.Rect(c.lay.Title.Min.X+gridPickerPad, c.lay.Title.Min.Y,
		c.lay.Title.Max.X, c.lay.Title.Max.Y), c.model.Title(), nil)
	if c.pages != nil {
		c.pages.Draw(dst)
	}

	// Left gutter: the label of each visible row.
	for row := 0; row < c.model.Rows(); row++ {
		r, visible := c.cellRect(0, row)
		if !visible {
			continue
		}
		label := c.model.RowLabel(row)
		lw := StyledTextWidth(label, RoleCaption)
		lx := c.lay.Grid.Min.X + (c.lay.Cells.Min.X-c.lay.Grid.Min.X-lw)/2
		ly := r.Min.Y + (r.Dy()-StyledTextHeight(RoleCaption))/2
		DrawTextStyled(dst, label, lx, ly, RoleCaption, colTextSecondary)
	}
}

// DrawOverlay paints what must sit ABOVE the cell buttons: the scrollbar and
// the close button.
func (c *GridPickerComponent) DrawOverlay(dst *ebiten.Image) {
	if !c.open || c.bounds.Empty() {
		return
	}
	if c.scroll.HasScroll() {
		c.scroll.Draw(dst)
	}
	if c.closeBtn != nil {
		c.closeBtn.Draw(dst)
	}
}
