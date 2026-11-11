package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Glue between the node sidebar's Pitch section and the note-matrix picker.
//
// The picker is a NoteMatrixComponent — the same component shape as the colour
// picker, sharing solveSwatchGrid — but it is driven from here rather than
// from the DrumView portal. The sidebar deliberately renders through GridTree
// (see NodeSidebar.Open), so a portal-hosted picker would paint over the
// bottom drum panel instead of being clipped to the grid pane.

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

// nodeInstrumentPitchOffset returns the live pitch offset of the instrument on
// the node's row, and whether it is pitched at all. Same resolution
// nodeNoteLabel uses, so the picker's labels and the chip's label cannot
// disagree — and since both now render through midiNoteName(nodePitchMIDI(...)),
// they cannot disagree on the NAME either (they briefly could have: the chip
// used to clamp to the piano range and the picker never did).
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
		// Only genuinely unpitched noise (hi-hats, claps, shaker, whisper) lands
		// here now that note display is universal — tuned percussion opens the
		// picker like anything else. The chip is not drawn for those rows
		// either, so this is belt-and-braces.
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
			// Identical mutation sequence to the pit- / pit+ handlers, so a
			// matrix pick is indistinguishable from a nudge to undo, the
			// predictor and the hooks bus.
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
	// Re-lay out immediately so the cell rects and their Buttons exist on THIS
	// frame. Without it the picker is drawn but inert until the next layout
	// pass, which is a frame of dead clicks right after it opens.
	sb.layout()

	// Start the keyboard cursor ON the current note, so the first arrow press
	// moves relative to where the user already is rather than jumping in from a
	// corner — and so Enter straight after opening is a no-op on the value.
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

// noteMatrixBounds is the container the picker is clamped inside: the grid
// pane, since that is what the sidebar renders into.
func (sb *NodeSidebar) noteMatrixBounds() image.Rectangle {
	g := sb.game
	if g == nil {
		return image.Rectangle{}
	}
	b := image.Rect(0, 0, g.winW, g.winH)
	if g.split != nil {
		b = image.Rect(0, 0, g.split.GridW(g.winW), g.split.GridH(g.winH))
	}
	// On a phone the picker spans the pane's width, so it starts BELOW the
	// top-right button row: those buttons (higher z) otherwise covered its
	// header, and with mobile's no-× design the strip beside them is the
	// "outside" a tap closes it from. Desktop pickers sit beside the card,
	// clear of the cluster, and keep the full pane height.
	if Profile().IsMobile() {
		if r := g.gridTopButtonsReservedRect(); !r.Empty() && r.Max.Y > b.Min.Y && r.Max.Y < b.Max.Y {
			b.Min.Y = r.Max.Y
		}
	}
	return b
}

// syncNoteMatrixProps refreshes the props that change while the picker is
// open: the anchor (the sidebar re-lays out every frame, so the chip moves
// with scroll and drag) and the current pitch (so -/+ nudges made with the
// picker open move the selected ring).
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

// noteMatrixCellAt returns the id of the selectable cell under a point; ok is
// false when the point is not on one.
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

// handleNoteMatrixInput gives the open picker first refusal on pointer input,
// with ONE exception: a press on a selectable cell falls through so the
// sidebar's normal Button path fires it (hover, press animation, deferred tap).
//
// Everything else inside the panel is absorbed here. The picker is modal over
// its own rect, and without that a press on a DIMMED cell reached whatever
// sidebar control it floats over — clicking an out-of-range cell nudged the
// pitch via the "−" key underneath it.
func (sb *NodeSidebar) handleNoteMatrixInput(x, y int, pressed bool) InputResult {
	if !sb.noteMatrixIsOpen() {
		return InputIgnored
	}
	sb.syncNoteMatrixProps()
	if _, onCell := sb.noteMatrixCellAt(x, y); onCell {
		return InputIgnored // let the cell Button handle it
	}
	// Anchor pass-through: the panel can cover the note chip that opened it, so
	// a press there must reach the chip Button (which toggles the picker shut)
	// rather than being absorbed as a chrome press. Same affordance the slider
	// popup gives its anchor.
	if a := sb.rects[sbPitchNote]; !a.Empty() && image.Pt(x, y).In(a) {
		return InputIgnored
	}
	return sb.noteMatrix.HandleInput(x, y, pressed)
}

// noteMatrixCellIDs are the sidebar button ids for the picker's visible cells,
// highest z first. buttonOrder() prepends these so a cell outranks every
// control it floats over.
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

// layoutNoteMatrixCells publishes each visible, selectable cell's rect into
// sb.rects so wireButtons can turn it into a Button. Stale ids from a previous
// scroll position are removed, otherwise a button would linger over a cell
// that has moved.
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
	// Drop buttons whose cell is gone (scrolled away or now out of range);
	// mk() only deletes ids it is asked about.
	for id := range sb.btns {
		if id.el == elNoteCell {
			if _, ok := sb.rects[id]; !ok {
				delete(sb.btns, id)
			}
		}
	}
}

// wireNoteMatrixCells registers one Button per selectable visible cell, using
// the sidebar's own mk() so cells get the shared keycap style, hover, press
// animation and deferred-tap dispatch.
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

// drawNoteMatrix paints the picker above the sidebar's own content: chrome,
// then the cell Buttons, then the scrollbar and close button.
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
		// Highlights are FILLS, not outlines — a 1px border is easy to lose on
		// a dense grid of keycaps.
		//
		// They paint ON TOP of the button, deliberately. Drawn underneath they
		// were invisible: the keycap face is opaque and covered them
		// completely, so the draw call happened and the pixels never changed
		// (a draw-call-counting test passed on exactly that). A translucent
		// tint over the cap reads as highlighted while leaving the note name
		// legible.
		//
		// The two states are separable because they can sit on different cells:
		// the accent tint marks the note the node currently sounds, the focus
		// tint marks where the keyboard cursor is.
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

// ─── Keyboard navigation ───────────────────────────────────────────────────
//
// The picker reuses MenuKeyNav, the same cursor the instrument / template /
// context menus use, via its opt-in grid interface: Up/Down move a whole row,
// Left/Right one cell, Enter picks. Home/End jump to the first/last selectable
// note and PageUp/PageDown move by a visible page — all inherited, none of it
// re-implemented here.

// noteMatrixNavHost adapts the picker to MenuKeyNavHost. Items are the FULL
// model grid in row-major order (index = row*cols + col), including the
// out-of-range cells, so the index arithmetic stays a plain stride; those cells
// simply report themselves unselectable and the cursor steps over them.
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

// MenuScrollBehavior is nil: the picker scrolls in ROWS while the cursor counts
// CELLS, so MenuRevealItem does the placing instead (MenuKeyNavRevealer).
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

// updateNoteMatrixKeyNav pumps the keyboard cursor for one frame. Called from
// the sidebar's per-frame update so arrows work whether or not the pointer is
// over the picker.
func (sb *NodeSidebar) updateNoteMatrixKeyNav() {
	if !sb.noteMatrixIsOpen() {
		if sb.noteMatrixKeyNav != nil {
			sb.noteMatrixKeyNav.Reset()
		}
		return
	}
	sb.noteMatrixNav().Update(noteMatrixNavHost{sb: sb})
}

// applyNoteMatrixPitch is the ONE mutation both a cell click and Enter go
// through, so keyboard and pointer cannot diverge.
//
// SELECTING APPLIES, RE-SELECTING CONFIRMS. Choosing a different note applies
// it and leaves the picker OPEN, so several notes can be auditioned without
// reopening the panel between each. Choosing the note that is already current
// is the confirm gesture and closes.
//
// The rule is positional, not time-based: there is no double-click timer to
// tune, and pointer and keyboard behave identically. The colour picker follows
// the same rule (drumview_overlay_color_comp.go).
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

	// Move the keyboard cursor onto the note just applied, so pointer and
	// keyboard stay coherent: after clicking a cell, Enter CONFIRMS it rather
	// than re-applying whatever the cursor was left pointing at.
	if col, row, ok := sb.noteMatrix.Model().CellFor(pitch); ok {
		if cols := sb.noteMatrix.Model().Cols(); cols > 0 {
			nav := sb.noteMatrixNav()
			nav.BindHost(noteMatrixNavHost{sb: sb})
			nav.SetSelected(row*cols + col)
		}
	}

	// Stay open, and re-lay out so the highlight, the chip label and the cell
	// buttons all reflect the note just applied.
	sb.layout()
}

// noteMatrixKeyCursorID is the sidebar button id under the keyboard cursor, or
// the zero sbID when nothing is highlighted. Draw uses it to render the same focus ring a
// hovered cell gets, so the keyboard cursor is visible.
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
