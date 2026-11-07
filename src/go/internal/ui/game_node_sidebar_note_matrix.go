package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Driven from here, not the DrumView portal: the sidebar renders through GridTree, and a portal
// picker would paint over the drum panel instead of being clipped to the grid pane.

// noteMatrixIsOpen reports whether the picker is showing.
func (sb *NodeSidebar) noteMatrixIsOpen() bool {
	return sb.noteMatrix != nil && sb.noteMatrix.IsOpen()
}

// closeNoteMatrix hides the picker. Safe when it was never opened.
func (sb *NodeSidebar) closeNoteMatrix() {
	if sb.noteMatrix != nil {
		sb.noteMatrix.Close()
	}
}

// Same resolution as nodeNoteLabel, so the picker and the chip cannot disagree on a note.
func (sb *NodeSidebar) nodeInstrumentPitchOffset() (float64, bool) {
	if sb.node == nil || sb.game == nil {
		return 0, false
	}
	g := sb.game
	row, ok := g.nodeRows[sb.node.ID]
	if !ok || row < 0 || row >= len(g.drum.Rows) {
		return 0, false
	}
	return audio.InstrumentPitchInfo(g.drum.Rows[row].Instrument)
}

// openNoteMatrix opens the picker anchored to the note chip.
func (sb *NodeSidebar) openNoteMatrix() {
	if sb.node == nil || sb.game == nil {
		return
	}
	offset, pitched := sb.nodeInstrumentPitchOffset()
	if !pitched {
		// Only unpitched noise lands here; its chip is not drawn either.
		return
	}
	g := sb.game
	mn, ok := g.graph.GetNodeByID(sb.node.ID)
	if !ok {
		return
	}
	if sb.noteMatrix == nil {
		sb.noteMatrix = NewNoteMatrixComponent()
	}

	node := sb.node
	sb.noteMatrix.SetProps(NoteMatrixProps{
		AnchorRect:       sb.rects[sbPitchNote],
		Bounds:           sb.noteMatrixBounds(),
		CurrentPitch:     mn.Params.Pitch,
		InstrumentOffset: offset,
		OnPick: func(pitch float64) {
			// Identical to the pit-/pit+ handlers, so undo, the predictor and the bus cannot tell them apart.
			g.enqueueUI(func() {
				if mn, ok := g.graph.GetNodeByID(node.ID); ok {
					p := mn.Params
					p.Pitch = pitch
					g.graph.SetNodeParams(node.ID, p)
					g.notifyPredictorNode(node.ID)
					emitNodeParamsChanged(node.ID, p)
				}
			})
		},
		OnClose: func() {},
	})
	sb.noteMatrix.Open()
	// Now, so the cell Buttons exist this frame; otherwise the first frame of clicks is dead.
	sb.layout()

	// On the current note, so the first arrow is relative and an immediate Enter changes nothing.
	host := noteMatrixNavHost{sb: sb}
	nav := sb.noteMatrixNav()
	nav.BindHost(host)
	nav.Reset()
	if col, row, ok := sb.noteMatrix.Model().CellFor(mn.Params.Pitch); ok {
		if cols := sb.noteMatrix.Model().Cols(); cols > 0 {
			nav.SetSelected(row*cols + col)
		}
	}
}

func (sb *NodeSidebar) noteMatrixBounds() image.Rectangle {
	g := sb.game
	if g == nil {
		return image.Rectangle{}
	}
	b := image.Rect(0, 0, g.winW, g.winH)
	if g.split != nil {
		b = image.Rect(0, 0, g.split.GridW(g.winW), g.split.GridH(g.winH))
	}
	// On a phone it spans the pane, so it starts below the top-right buttons: they would cover its
	// header, and the strip beside them is the "outside" a tap closes it from.
	if Profile().IsMobile() {
		if r := g.gridTopButtonsReservedRect(); !r.Empty() && r.Max.Y > b.Min.Y && r.Max.Y < b.Max.Y {
			b.Min.Y = r.Max.Y
		}
	}
	return b
}

// The sidebar re-lays out every frame (the chip moves), and -/+ nudges must move the ring.
func (sb *NodeSidebar) syncNoteMatrixProps() {
	if !sb.noteMatrixIsOpen() || sb.node == nil || sb.game == nil {
		return
	}
	p := sb.noteMatrix.Props()
	p.AnchorRect = sb.rects[sbPitchNote]
	if mn, ok := sb.game.graph.GetNodeByID(sb.node.ID); ok {
		p.CurrentPitch = mn.Params.Pitch
	}
	sb.noteMatrix.SetProps(p)
}

func (sb *NodeSidebar) noteMatrixCellAt(x, y int) (sbID, bool) {
	if !sb.noteMatrixIsOpen() {
		return sbID{}, false
	}
	pt := image.Pt(x, y)
	for _, c := range sb.noteMatrix.VisibleCells() {
		if c.Selectable && pt.In(c.Rect) {
			return c.ID, true
		}
	}
	return sbID{}, false
}

// Absorbs everything except presses on selectable cells, which fall through to the Button path.
// Modal over its rect: otherwise a dimmed cell's press reached the control beneath it.
func (sb *NodeSidebar) handleNoteMatrixInput(x, y int, pressed bool) InputResult {
	if !sb.noteMatrixIsOpen() {
		return InputIgnored
	}
	sb.syncNoteMatrixProps()
	if _, onCell := sb.noteMatrixCellAt(x, y); onCell {
		return InputIgnored // let the cell Button handle it
	}
	// The panel may cover the chip that opened it; a press there must reach the chip, which closes it.
	if a := sb.rects[sbPitchNote]; !a.Empty() && image.Pt(x, y).In(a) {
		return InputIgnored
	}
	return sb.noteMatrix.HandleInput(x, y, pressed)
}

// Highest z first, so a cell outranks every control it floats over.
func (sb *NodeSidebar) noteMatrixCellIDs() []sbID {
	if !sb.noteMatrixIsOpen() {
		return nil
	}
	cells := sb.noteMatrix.VisibleCells()
	out := make([]sbID, 0, len(cells))
	for _, c := range cells {
		if c.Selectable {
			out = append(out, c.ID)
		}
	}
	return out
}

// Stale ids are removed, or a button lingers over a cell that moved.
func (sb *NodeSidebar) layoutNoteMatrixCells() {
	for id := range sb.rects {
		if id.el == elNoteCell {
			delete(sb.rects, id)
		}
	}
	if !sb.noteMatrixIsOpen() {
		for id := range sb.btns {
			if id.el == elNoteCell {
				delete(sb.btns, id)
			}
		}
		return
	}
	for _, c := range sb.noteMatrix.VisibleCells() {
		if c.Selectable {
			sb.rects[c.ID] = c.Rect
		}
	}
	// mk() only deletes ids it is asked about.
	for id := range sb.btns {
		if id.el == elNoteCell {
			if _, ok := sb.rects[id]; !ok {
				delete(sb.btns, id)
			}
		}
	}
}

// Through the sidebar's mk(), so cells get the shared style, hover, press and deferred tap.
func (sb *NodeSidebar) wireNoteMatrixCells(mk func(id sbID, label string, onClick func())) {
	if !sb.noteMatrixIsOpen() || sb.node == nil || sb.game == nil {
		return
	}
	for _, c := range sb.noteMatrix.VisibleCells() {
		if !c.Selectable {
			continue
		}
		pitch := c.Pitch
		mk(c.ID, c.Label, func() { sb.applyNoteMatrixPitch(pitch) })
	}
}

func (sb *NodeSidebar) drawNoteMatrix(dst *ebiten.Image) {
	if !sb.noteMatrixIsOpen() {
		return
	}
	sb.syncNoteMatrixProps()
	sb.noteMatrix.DrawChrome(dst)

	cursorID := sb.noteMatrixKeyCursorID()
	for _, c := range sb.noteMatrix.VisibleCells() {
		if !c.Selectable {
			// Out-of-range cell: faded, no button, not tappable.
			drawRoundedRect(dst, c.Rect, WithAlpha(colSurface1, AlphaFaint), fitRadius(RadiusControl, c.Rect), true)
			continue
		}
		if b := sb.btns[c.ID]; b != nil {
			b.Draw(dst)
		}
		// Fills, painted ON TOP of the button: under the opaque keycap they never changed a pixel.
		// Accent = the note the node sounds; focus = the keyboard cursor, which may sit elsewhere.
		if c.Selected {
			drawRoundedRect(dst, c.Rect, WithAlpha(colAccent, AlphaMedium), fitRadius(RadiusControl, c.Rect), true)
			drawRoundedRect(dst, c.Rect, colAccent, fitRadius(RadiusControl, c.Rect), false)
		}
		if c.ID == cursorID {
			drawRoundedRect(dst, c.Rect, WithAlpha(genColorFocusRing, AlphaMedium), fitRadius(RadiusControl, c.Rect), true)
			drawRoundedRect(dst, c.Rect.Inset(-2), WithAlpha(genColorFocusRing, genAlphaStrong), fitRadius(RadiusControl, c.Rect), false)
		}
	}

	sb.noteMatrix.DrawOverlay(dst)
}

// Reuses MenuKeyNav's grid mode: Up/Down a row, Left/Right a cell, Home/End, paging, Enter.

// The FULL grid including out-of-range cells, so the index is a plain stride; those cells
// report unselectable and the cursor skips them.
type noteMatrixNavHost struct{ sb *NodeSidebar }

func (h noteMatrixNavHost) model() (noteMatrixModel, bool) {
	if !h.sb.noteMatrixIsOpen() {
		return noteMatrixModel{}, false
	}
	return h.sb.noteMatrix.Model(), true
}

func (h noteMatrixNavHost) MenuGridCols() int {
	m, ok := h.model()
	if !ok {
		return 0
	}
	return m.Cols()
}

func (h noteMatrixNavHost) MenuItemCount() int {
	m, ok := h.model()
	if !ok {
		return 0
	}
	return m.Cols() * noteMatrixRows
}

func (h noteMatrixNavHost) cellAt(i int) (col, row int, ok bool) {
	m, open := h.model()
	if !open || m.Cols() <= 0 || i < 0 || i >= m.Cols()*noteMatrixRows {
		return 0, 0, false
	}
	return i % m.Cols(), i / m.Cols(), true
}

func (h noteMatrixNavHost) MenuItemSelectable(i int) bool {
	col, row, ok := h.cellAt(i)
	if !ok {
		return false
	}
	m, _ := h.model()
	_, selectable := m.PitchAt(col, row)
	return selectable
}

func (h noteMatrixNavHost) MenuActivate(i int) {
	col, row, ok := h.cellAt(i)
	if !ok {
		return
	}
	m, _ := h.model()
	pitch, selectable := m.PitchAt(col, row)
	if !selectable {
		return
	}
	h.sb.applyNoteMatrixPitch(pitch)
}

// nil: the picker scrolls in rows while the cursor counts cells, so MenuRevealItem places it.
func (h noteMatrixNavHost) MenuScrollBehavior() *ScrollBehavior { return nil }

// MenuRevealItem scrolls the cursor's ROW into the visible window.
func (h noteMatrixNavHost) MenuRevealItem(i int) {
	_, row, ok := h.cellAt(i)
	if !ok {
		return
	}
	h.sb.noteMatrix.RevealRow(row)
}

// noteMatrixNav lazily creates the picker's keyboard cursor.
func (sb *NodeSidebar) noteMatrixNav() *MenuKeyNav {
	if sb.noteMatrixKeyNav == nil {
		sb.noteMatrixKeyNav = NewMenuKeyNav()
	}
	return sb.noteMatrixKeyNav
}

// From the per-frame update, so arrows work wherever the pointer is.
func (sb *NodeSidebar) updateNoteMatrixKeyNav() {
	if !sb.noteMatrixIsOpen() {
		if sb.noteMatrixKeyNav != nil {
			sb.noteMatrixKeyNav.Reset()
		}
		return
	}
	sb.noteMatrixNav().Update(noteMatrixNavHost{sb: sb})
}

// The one mutation for click and Enter. A new note applies and keeps the picker open for
// auditioning; the current note confirms and closes. Positional, not timed.
func (sb *NodeSidebar) applyNoteMatrixPitch(pitch float64) {
	if sb.node == nil || sb.game == nil {
		return
	}
	g := sb.game
	id := sb.node.ID
	mn, ok := g.graph.GetNodeByID(id)
	if !ok {
		return
	}
	if mn.Params.Pitch == pitch {
		// Already applied — this is the confirm.
		sb.closeNoteMatrix()
		return
	}
	p := mn.Params
	p.Pitch = pitch
	g.graph.SetNodeParams(id, p)
	g.notifyPredictorNode(id)
	emitNodeParamsChanged(id, p)

	// So after a click, Enter confirms that note.
	if col, row, ok := sb.noteMatrix.Model().CellFor(pitch); ok {
		if cols := sb.noteMatrix.Model().Cols(); cols > 0 {
			nav := sb.noteMatrixNav()
			nav.BindHost(noteMatrixNavHost{sb: sb})
			nav.SetSelected(row*cols + col)
		}
	}

	sb.layout()
}

func (sb *NodeSidebar) noteMatrixKeyCursorID() sbID {
	if !sb.noteMatrixIsOpen() || sb.noteMatrixKeyNav == nil {
		return sbID{}
	}
	sel := sb.noteMatrixKeyNav.Selected()
	if sel < 0 {
		return sbID{}
	}
	h := noteMatrixNavHost{sb: sb}
	col, row, ok := h.cellAt(sel)
	if !ok {
		return sbID{}
	}
	return sbNoteCell(col, row)
}
