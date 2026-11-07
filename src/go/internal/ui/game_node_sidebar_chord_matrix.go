package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Wired like the note matrix: the sidebar owns the picker (GridTree, not the portal), cells are
// real sidebar Buttons, and selecting applies while re-selecting confirms.

// chordMatrixCellPrefix spells a chord-picker cell in sbID.String.
const chordMatrixCellPrefix = "chordcell:"

// chordMatrixIsOpen reports whether the chord picker is showing.
func (sb *NodeSidebar) chordMatrixIsOpen() bool {
	return sb.chordMatrix != nil && sb.chordMatrix.IsOpen()
}

// closeChordMatrix hides the chord picker. Safe when it was never opened.
func (sb *NodeSidebar) closeChordMatrix() {
	if sb.chordMatrix != nil {
		sb.chordMatrix.Close()
	}
}

// Routing asks this rather than naming a picker.
func (sb *NodeSidebar) openPicker() *GridPickerComponent {
	switch {
	case sb.noteMatrixIsOpen():
		return sb.noteMatrix.GridPickerComponent
	case sb.chordMatrixIsOpen():
		return sb.chordMatrix.GridPickerComponent
	}
	return nil
}

func (sb *NodeSidebar) openChordMatrix(anchor sbID) {
	if sb.node == nil || sb.game == nil {
		return
	}
	mn, ok := sb.game.graph.GetNodeByID(sb.node.ID)
	if !ok {
		return
	}
	sb.closeNoteMatrix()
	if sb.chordMatrix == nil {
		sb.chordMatrix = NewChordMatrixComponent()
	}
	offset, _ := sb.nodeInstrumentPitchOffset()
	sb.chordMatrixAnchor = anchor
	sb.chordMatrix.OnPageChange = sb.reseatChordMatrixCursor
	sb.chordMatrix.Open(sb.rects[anchor], sb.noteMatrixBounds(), mn.Params.Pitch, offset, mn.Params.Chord, func() {})
	sb.layout()

	nav := sb.chordMatrixNav()
	nav.BindHost(chordMatrixNavHost{sb: sb})
	nav.Reset()
	sb.reseatChordMatrixCursor()
}

// A cursor carried across a page switch at the same (col, row) tinted a chord the node was not
// playing and read as the selection.
func (sb *NodeSidebar) reseatChordMatrixCursor() {
	if !sb.chordMatrixIsOpen() {
		return
	}
	nav := sb.chordMatrixNav()
	m := sb.chordMatrix.Model()
	if col, row, ok := m.Selected(); ok {
		nav.SetSelected(row*m.Cols() + col)
		return
	}
	nav.SetSelected(MenuKeyNavNoSelection)
}

// syncChordMatrixProps refreshes the anchor and the node's current chord.
func (sb *NodeSidebar) syncChordMatrixProps() {
	if !sb.chordMatrixIsOpen() || sb.node == nil || sb.game == nil {
		return
	}
	if mn, ok := sb.game.graph.GetNodeByID(sb.node.ID); ok {
		sb.chordMatrix.Sync(sb.rects[sb.chordMatrixAnchor], mn.Params.Pitch, mn.Params.Chord)
	}
}

// chordMatrixCellAt is the id of the cell under a point; ok is false off a cell.
func (sb *NodeSidebar) chordMatrixCellAt(x, y int) (sbID, bool) {
	if !sb.chordMatrixIsOpen() {
		return sbID{}, false
	}
	pt := image.Pt(x, y)
	for _, c := range sb.chordMatrix.visibleCells() {
		if c.Selectable && pt.In(c.Rect) {
			return sbChordCell(c.Col, c.Row), true
		}
	}
	return sbID{}, false
}

// Cells and the anchor chip pass through: their Buttons fire them.
func (sb *NodeSidebar) handleChordMatrixInput(x, y int, pressed bool) InputResult {
	if !sb.chordMatrixIsOpen() {
		return InputIgnored
	}
	sb.syncChordMatrixProps()
	if _, onCell := sb.chordMatrixCellAt(x, y); onCell {
		return InputIgnored
	}
	for _, id := range []sbID{sbChordName, sbPitchNote} { // either chip toggles it shut
		if a := sb.rects[id]; !a.Empty() && image.Pt(x, y).In(a) {
			return InputIgnored
		}
	}
	r := sb.chordMatrix.HandleInput(x, y, pressed)
	if r != InputIgnored {
		sb.layoutChordMatrixCells() // a page switch relabels the cells now
	}
	return r
}

// chordMatrixCellIDs are the visible cells' button ids (for buttonOrder).
func (sb *NodeSidebar) chordMatrixCellIDs() []sbID {
	if !sb.chordMatrixIsOpen() {
		return nil
	}
	cells := sb.chordMatrix.visibleCells()
	out := make([]sbID, 0, len(cells))
	for _, c := range cells {
		if c.Selectable {
			out = append(out, sbChordCell(c.Col, c.Row))
		}
	}
	return out
}

// layoutChordMatrixCells publishes the visible cells' rects (screen space).
func (sb *NodeSidebar) layoutChordMatrixCells() {
	for id := range sb.rects {
		if id.el == elChordCell {
			delete(sb.rects, id)
		}
	}
	if sb.chordMatrixIsOpen() {
		for _, c := range sb.chordMatrix.visibleCells() {
			if c.Selectable {
				sb.rects[sbChordCell(c.Col, c.Row)] = c.Rect
			}
		}
	}
	for id := range sb.btns {
		if id.el == elChordCell {
			if _, ok := sb.rects[id]; !ok {
				delete(sb.btns, id)
			}
		}
	}
}

func (sb *NodeSidebar) wireChordMatrixCells(mk func(id sbID, label string, onClick func())) {
	if !sb.chordMatrixIsOpen() || sb.node == nil || sb.game == nil {
		return
	}
	for _, c := range sb.chordMatrix.visibleCells() {
		if !c.Selectable {
			continue
		}
		col, row := c.Col, c.Row
		id := sbChordCell(col, row)
		mk(id, c.Label, func() { sb.applyChordMatrixPick(col, row) })
		// Measured, not counted, so "C°" and "Emaj9" stay large.
		if b := sb.btns[id]; b != nil && StyledTextWidth(c.Label, RoleBody) > c.Rect.Dx()-8 {
			b.SetTextRole(RoleCaption)
		}
	}
}

// Tints ON TOP of the cells: under an opaque keycap they are invisible.
func (sb *NodeSidebar) drawChordMatrix(dst *ebiten.Image) {
	if !sb.chordMatrixIsOpen() {
		return
	}
	sb.syncChordMatrixProps()
	sb.chordMatrix.DrawChrome(dst)
	cursorID := sb.chordMatrixKeyCursorID()
	for _, c := range sb.chordMatrix.visibleCells() {
		id := sbChordCell(c.Col, c.Row)
		if b := sb.btns[id]; b != nil {
			b.Draw(dst)
		}
		if c.Selected {
			drawRoundedRect(dst, c.Rect, WithAlpha(colAccent, AlphaMedium), fitRadius(RadiusControl, c.Rect), true)
			drawRoundedRect(dst, c.Rect, colAccent, fitRadius(RadiusControl, c.Rect), false)
		}
		if id == cursorID {
			drawRoundedRect(dst, c.Rect, WithAlpha(genColorFocusRing, AlphaMedium), fitRadius(RadiusControl, c.Rect), true)
			drawRoundedRect(dst, c.Rect.Inset(-2), WithAlpha(genColorFocusRing, genAlphaStrong), fitRadius(RadiusControl, c.Rect), false)
		}
	}
	sb.chordMatrix.DrawOverlay(dst)
}

// The one mutation for click and Enter: root + spec in ONE SetNodeParams (one undo step); the
// current chord closes the picker.
func (sb *NodeSidebar) applyChordMatrixPick(col, row int) {
	if sb.node == nil || sb.game == nil || !sb.chordMatrixIsOpen() {
		return
	}
	g := sb.game
	id := sb.node.ID
	mn, ok := g.graph.GetNodeByID(id)
	if !ok {
		return
	}
	sb.syncChordMatrixProps()
	pitch, spec := sb.chordMatrix.Model().PickAt(col, row)
	cur := model.NormalizeChord(mn.Params.Chord)
	if cur.IsZero() {
		cur = model.ChordSpec{Quality: model.ChordDefaultQuality}
	}
	if pitch == mn.Params.Pitch && spec == cur {
		sb.closeChordMatrix() // already applied — this is the confirm
		return
	}
	p := mn.Params
	p.Pitch = pitch
	p.Chord = spec
	g.graph.SetNodeParams(id, p)
	g.notifyPredictorNode(id)
	emitNodeChordChanged(id, p, g.nodeChordLabel(id))

	nav := sb.chordMatrixNav()
	nav.BindHost(chordMatrixNavHost{sb: sb})
	nav.SetSelected(row*sb.chordMatrix.Model().Cols() + col)
	sb.layout()
}

// ─── Keyboard navigation ───────────────────────────────────────────────────

type chordMatrixNavHost struct{ sb *NodeSidebar }

func (h chordMatrixNavHost) cols() int {
	if !h.sb.chordMatrixIsOpen() {
		return 0
	}
	return h.sb.chordMatrix.Model().Cols()
}

func (h chordMatrixNavHost) MenuGridCols() int { return h.cols() }
func (h chordMatrixNavHost) MenuItemCount() int {
	if !h.sb.chordMatrixIsOpen() {
		return 0
	}
	return h.cols() * h.sb.chordMatrix.Model().Rows()
}
func (h chordMatrixNavHost) MenuItemSelectable(i int) bool { return i >= 0 && i < h.MenuItemCount() }
func (h chordMatrixNavHost) MenuActivate(i int) {
	if c := h.cols(); c > 0 && h.MenuItemSelectable(i) {
		h.sb.applyChordMatrixPick(i%c, i/c)
	}
}
func (h chordMatrixNavHost) MenuScrollBehavior() *ScrollBehavior { return nil }
func (h chordMatrixNavHost) MenuRevealItem(i int) {
	if c := h.cols(); c > 0 {
		h.sb.chordMatrix.RevealRow(i / c)
	}
}

func (sb *NodeSidebar) chordMatrixNav() *MenuKeyNav {
	if sb.chordMatrixKeyNav == nil {
		sb.chordMatrixKeyNav = NewMenuKeyNav()
	}
	return sb.chordMatrixKeyNav
}

// Tab = next page, Shift+Tab = previous.
func (sb *NodeSidebar) updateChordMatrixKeyNav() {
	if !sb.chordMatrixIsOpen() {
		if sb.chordMatrixKeyNav != nil {
			sb.chordMatrixKeyNav.Reset()
		}
		return
	}
	// Our own held flag, so a held Tab turns one page.
	tab := isKeyPressed(ebiten.KeyTab)
	if tab && !sb.chordTabHeld {
		step := 1
		if menuKeyNavShiftHeld() {
			step = -1
		}
		sb.chordMatrix.SetPage(sb.chordMatrix.Page() + step)
		sb.layout()
	}
	sb.chordTabHeld = tab
	sb.chordMatrixNav().Update(chordMatrixNavHost{sb: sb})
}

// chordMatrixKeyCursorID is the button id under the keyboard cursor.
func (sb *NodeSidebar) chordMatrixKeyCursorID() sbID {
	if !sb.chordMatrixIsOpen() || sb.chordMatrixKeyNav == nil {
		return sbID{}
	}
	sel := sb.chordMatrixKeyNav.Selected()
	c := chordMatrixNavHost{sb: sb}.cols()
	if sel < 0 || c <= 0 {
		return sbID{}
	}
	return sbChordCell(sel%c, sel/c)
}
