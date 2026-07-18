package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// nodeClipboard is an in-memory, single-node clipboard: a deep, independent
// snapshot of a node's type + params. It holds no NodeID (paste mints a fresh
// one), no grid position (paste uses the cursor-guide cell), and no edges
// (params-only paste). The zero value is an empty clipboard.
type nodeClipboard struct {
	has    bool
	typ    model.NodeType
	params model.NodeParams
}

// cloneNodeParams returns a deep copy of p with the non-serializable Logic func
// dropped (LogicKind/LogicN/LogicP carry the rule) and the EffectOverrides
// slice cloned, so later edits to the source node cannot mutate the clipboard.
func cloneNodeParams(p model.NodeParams) model.NodeParams {
	c := p
	c.Logic = nil
	if len(p.EffectOverrides) > 0 {
		c.EffectOverrides = make([]model.EffectOverride, len(p.EffectOverrides))
		copy(c.EffectOverrides, p.EffectOverrides)
	} else {
		c.EffectOverrides = nil
	}
	return c
}

// snapshotNodeToClipboard captures n's authoritative model type + params into
// g.clipboard. Returns false (no capture) if n is nil or has no model node.
func (g *Game) snapshotNodeToClipboard(n *uiNode) bool {
	if n == nil {
		return false
	}
	node, ok := g.graph.GetNodeByID(n.ID)
	if !ok {
		return false
	}
	g.clipboard = nodeClipboard{has: true, typ: node.Type, params: cloneNodeParams(node.Params)}
	return true
}

// copySelectedNode snapshots the currently selected node into the clipboard and
// emits EventNodeCopied. No-op when nothing is selected.
func (g *Game) copySelectedNode() {
	if g.sel == nil {
		return
	}
	if !g.snapshotNodeToClipboard(g.sel) {
		return
	}
	emitNodeCopied(g.sel.ID, g.sel.I, g.sel.J, g.clipboard.typ)
}

// clipboardShortcut handles Ctrl/Cmd-C/X/V. Ungated router node: it self-gates
// on the modifier so plain C/X/V never trigger it. Returns whether it acted.
func (g *Game) clipboardShortcut() bool {
	ctrl := isKeyPressed(ebiten.KeyControlLeft) || isKeyPressed(ebiten.KeyControlRight) ||
		isKeyPressed(ebiten.KeyMetaLeft) || isKeyPressed(ebiten.KeyMetaRight)
	if !ctrl {
		return false
	}
	switch {
	case isKeyJustPressed(ebiten.KeyC):
		g.copySelectedNode()
		return true
	case isKeyJustPressed(ebiten.KeyX):
		g.cutSelectedNode()
		return true
	case isKeyJustPressed(ebiten.KeyV):
		g.pasteAtCursor()
		return true
	}
	return false
}

// cutSelectedNode snapshots the selected node, emits the cut narrative, then
// deletes it (deleteNode is undoable and logs the delete). The clipboard
// snapshot survives the delete, so cut → paste-elsewhere behaves as a move.
func (g *Game) cutSelectedNode() {
	if g.sel == nil {
		return
	}
	n := g.sel
	if !g.snapshotNodeToClipboard(n) {
		return
	}
	emitNodeCut(n.ID, n.I, n.J, g.clipboard.typ)
	g.deleteNode(n)
}

// pasteNodeFrom mints a fresh node at (i,j) from cb: cb.typ + a deep copy of
// cb.params. Like manual node creation, if (i,j) lies on an existing orthogonal
// edge the pasted node is auto-stitched into that connection via the SAME
// stitchEdgesAt tryAddNode uses; off an edge it has no edges. This is
// deliberately NOT tryAddNode, whose new-node tail is entangled with
// origin/start-node logic that paste must not run. Selects the node, collapses
// to one undo step, emits EventNodePasted. Precondition: (i,j) is empty (caller
// clears any occupant).
func (g *Game) pasteNodeFrom(i, j int, cb nodeClipboard) *uiNode {
	if !cb.has {
		return nil
	}
	beginUndoGroup("paste node")
	defer endUndoGroup()

	id := g.graph.AddNode(i, j, cb.typ)
	if node, ok := g.graph.GetNodeByID(id); ok {
		node.Params = cloneNodeParams(cb.params)
		g.graph.Nodes[id] = node
	}
	g.cacheNode(id)

	unit := g.grid.Unit()
	n := &uiNode{ID: id, I: i, J: j, X: float64(i) * unit, Y: float64(j) * unit}
	g.nodes = append(g.nodes, n)
	g.nodesByID[n.ID] = n
	if !g.importing {
		g.notifyPredictorNode(id)
	}

	// Select the pasted node (consistent with manual add).
	if g.sel != nil {
		g.sel.Selected = false
	}
	g.sel = n
	n.Selected = true
	g.computeSelNeighbors()
	g.coordBadgeNode = n
	g.coordBadgeFrame = g.frame

	// Auto-stitch into an existing edge if this cell lies on one — the exact
	// same wiring manual node creation uses (tryAddNode → stitchEdgesAt). It is
	// a no-op when no edge passes through (i,j), so an off-edge paste stays
	// edge-free.
	if !g.importing {
		g.stitchEdgesAt(n)
	}

	if !g.importing {
		g.updateBeatInfos()
	}
	emitNodePasted(n.ID, i, j, cb.typ)
	return n
}

// pasteAtCursor resolves the cursor-guide cell and pastes the clipboard node
// there. Empty cell → immediate paste. Occupied cell → open the overwrite
// confirmation (Task 6). No-op with an empty clipboard.
func (g *Game) pasteAtCursor() {
	if !g.clipboard.has {
		return
	}
	x, y := cursorPosition()
	gt := g.guideTargetAt(x, y)
	i, j := gt.I, gt.J
	if existing := g.nodeAt(i, j); existing != nil {
		g.beginPasteConfirm(i, j)
		return
	}
	g.pasteNodeFrom(i, j, g.clipboard)
}

const pasteConfirmPortalID = "paste-confirm"

// pastePendingState holds an in-flight paste-overwrite confirmation. snap is the
// clipboard captured at paste time, so a copy issued while the dialog is open
// can't change what gets pasted.
type pastePendingState struct {
	active bool
	i, j   int
	snap   nodeClipboard
}

func (g *Game) pasteConfirmActive() bool { return g.pastePending.active }

// beginPasteConfirm arms the overwrite confirmation for cell (i,j) and opens the
// modal portal. The graph is NOT mutated until ConfirmPasteOverwrite.
func (g *Game) beginPasteConfirm(i, j int) {
	g.pastePending = pastePendingState{active: true, i: i, j: j, snap: g.clipboard}
	g.openPasteConfirmPortal()
}

// ConfirmPasteOverwrite deletes the occupant at the pending cell and pastes the
// captured snapshot, collapsed into one "replace node" undo step. Public so the
// portal button, Enter, and tests drive it uniformly.
func (g *Game) ConfirmPasteOverwrite() {
	if !g.pastePending.active {
		return
	}
	i, j, snap := g.pastePending.i, g.pastePending.j, g.pastePending.snap
	g.pastePending = pastePendingState{}
	g.closePasteConfirmPortal()

	beginUndoGroup("replace node")
	defer endUndoGroup()
	if existing := g.nodeAt(i, j); existing != nil {
		g.deleteNode(existing)
	}
	g.pasteNodeFrom(i, j, snap)
}

// CancelPasteOverwrite dismisses the confirmation without mutating the graph.
func (g *Game) CancelPasteOverwrite() {
	if !g.pastePending.active {
		return
	}
	g.pastePending = pastePendingState{}
	g.closePasteConfirmPortal()
}

// pasteConfirmRect centers the confirmation dialog over the grid content pane.
func (g *Game) pasteConfirmRect() image.Rectangle {
	grid := g.split.GridRect(g.winW, g.winH)
	top := gridTopOffset()
	cx := (grid.Min.X + grid.Max.X) / 2
	cy := top + (grid.Max.Y-top)/2
	w, h := 320, 140
	return image.Rect(cx-w/2, cy-h/2, cx+w/2, cy+h/2)
}

func (g *Game) pasteConfirmOverwriteBtnRect() image.Rectangle {
	r := g.pasteConfirmRect()
	btnW, btnH := 120, 32
	return image.Rect(r.Max.X-SpaceMD-btnW, r.Max.Y-SpaceMD-btnH, r.Max.X-SpaceMD, r.Max.Y-SpaceMD)
}

func (g *Game) pasteConfirmCancelBtnRect() image.Rectangle {
	r := g.pasteConfirmRect()
	btnW, btnH := 120, 32
	return image.Rect(r.Min.X+SpaceMD, r.Max.Y-SpaceMD-btnH, r.Min.X+SpaceMD+btnW, r.Max.Y-SpaceMD)
}

// openPasteConfirmPortal shows the modal confirmation. The pending state is the
// source of truth; if no portal is available (headless tests) the confirmation
// logic still works via Confirm/Cancel.
func (g *Game) openPasteConfirmPortal() {
	if g.drum == nil || g.drum.portal() == nil {
		return
	}
	g.drum.portal().Open(PortalEntry{
		ID:    pasteConfirmPortalID,
		Modal: true,
		Scrim: true,
		Overlay: &dvOverlayPortal{
			id:       pasteConfirmPortalID,
			isOpenFn: func() bool { return g.pastePending.active },
			rectFn:   func() image.Rectangle { return g.pasteConfirmRect() },
			drawFn:   func(dst *ebiten.Image) { g.drawPasteConfirm(dst) },
			inputFn: func(x, y int, pressed bool) InputResult {
				if !pressed {
					return InputConsumed
				}
				pt := image.Pt(x, y)
				if pt.In(g.pasteConfirmOverwriteBtnRect()) {
					g.ConfirmPasteOverwrite()
					return InputConsumed
				}
				if pt.In(g.pasteConfirmCancelBtnRect()) {
					g.CancelPasteOverwrite()
					return InputConsumed
				}
				return InputConsumed // swallow stray taps inside the modal
			},
			// Cleanup-only, matching the portalEscapeHandler contract (see
			// handleTemplateSearchEscape): clear our own state and return false so
			// handleEscape's CloseTop removes the (still-topmost) paste-confirm entry
			// itself. Do NOT close the portal here — CancelPasteOverwrite's Close(id)
			// would pop this entry from mid-stack and then handleEscape's fallback
			// CloseTop would wrongly close whatever portal sits beneath us.
			escapeFn: func() bool { g.pastePending = pastePendingState{}; return false },
		},
		OnClose: func() { g.pastePending = pastePendingState{} },
	})
}

func (g *Game) closePasteConfirmPortal() {
	if g.drum == nil || g.drum.portal() == nil {
		return
	}
	g.drum.portal().Close(pasteConfirmPortalID)
}

// drawPasteConfirm renders the "Replace node?" modal: panel + prompt + two
// buttons. Button chrome mirrors drawSaveAsDialog's rounded-rect style.
func (g *Game) drawPasteConfirm(dst *ebiten.Image) {
	if !g.pastePending.active {
		return
	}
	r := g.pasteConfirmRect()
	drawRoundedRect(dst, r, TokenSurface2(), RadiusMD, true)
	drawRoundedRect(dst, r, TokenBorderMedium(), RadiusMD, false)
	DrawTextStyled(dst, "Replace node?", r.Min.X+SpaceMD, r.Min.Y+SpaceMD, RolePanelTitle, TokenTextPrimary())

	ov := g.pasteConfirmOverwriteBtnRect()
	drawRoundedRect(dst, ov, TokenSurface3(), RadiusSM, true)
	DrawTextStyled(dst, "Overwrite", ov.Min.X+SpaceSM, ov.Min.Y+SpaceXS, RoleBody, TokenTextPrimary())

	cn := g.pasteConfirmCancelBtnRect()
	drawRoundedRect(dst, cn, TokenSurface3(), RadiusSM, true)
	DrawTextStyled(dst, "Cancel", cn.Min.X+SpaceSM, cn.Min.Y+SpaceXS, RoleBody, TokenTextPrimary())
}
