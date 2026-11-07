package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// No NodeID (paste mints one), no position (paste uses the guide cell), no edges. Zero = empty.
type nodeClipboard struct {
	has    bool
	typ    model.NodeType
	params model.NodeParams
}

// Deep copy without the Logic func (LogicKind/N/P carry the rule), so later edits to the source
// cannot change the clipboard.
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

func (g *Game) copySelectedNode() {
	if g.sel == nil {
		return
	}
	if !g.snapshotNodeToClipboard(g.sel) {
		return
	}
	emitNodeCopied(g.sel.ID, g.sel.I, g.sel.J, g.clipboard.typ)
}

// Ungated, self-gated on the modifier, so plain C/X/V never trigger it.
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

// The snapshot survives the delete, so cut then paste elsewhere is a move.
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

// Auto-stitched into an edge through the same stitchEdgesAt as manual creation, but deliberately not
// tryAddNode, whose origin/start-node logic paste must not run. One undo step; (i,j) must be empty.
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

	// A no-op off an edge, so an off-edge paste stays edge-free.
	if !g.importing {
		g.stitchEdgesAt(n)
	}

	if !g.importing {
		g.updateBeatInfos()
	}
	emitNodePasted(n.ID, i, j, cb.typ)
	return n
}

// An occupied cell opens the overwrite confirm.
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

// Captured at paste time, so a copy while the dialog is open cannot change what is pasted.
type pastePendingState struct {
	active bool
	i, j   int
	snap   nodeClipboard
}

func (g *Game) pasteConfirmActive() bool { return g.pastePending.active }

// Nothing changes until ConfirmPasteOverwrite.
func (g *Game) beginPasteConfirm(i, j int) {
	g.pastePending = pastePendingState{active: true, i: i, j: j, snap: g.clipboard}
	g.openPasteConfirmPortal()
}

// One "replace node" undo step. Public so the button, Enter and tests drive it the same way.
func (g *Game) ConfirmPasteOverwrite() {
	if !g.pastePending.active {
		return
	}
	i, j, snap := g.pastePending.i, g.pastePending.j, g.pastePending.snap
	g.pastePending = pastePendingState{}
	g.closePasteConfirmPortalIfNeeded()

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
	g.closePasteConfirmPortalIfNeeded()
}

// Not when the dialog already flagged itself closed: its own callback path is already removing the
// entry this frame, and a second pop would make Esc's fallback CloseTop close the portal beneath.
func (g *Game) closePasteConfirmPortalIfNeeded() {
	alreadyClosing := g.pasteConfirmDlg != nil && g.pasteConfirmDlg.ShouldClose()
	g.pasteConfirmDlg = nil
	if alreadyClosing {
		return
	}
	g.closePasteConfirmPortal()
}

// Shared by every grid-centred confirmation, so they centre the same way.
func (g *Game) gridPaneScreenRect() image.Rectangle {
	grid := g.split.GridRect(g.winW, g.winH)
	top := gridTopOffset()
	return image.Rect(grid.Min.X, top, grid.Max.X, grid.Max.Y)
}

// The pending state is the truth, so Confirm/Cancel work without a portal (headless tests).
func (g *Game) openPasteConfirmPortal() {
	if g.drum == nil || g.drum.portal() == nil {
		return
	}
	g.pasteConfirmDlg = newConfirmDialog(confirmConfig{
		id:          pasteConfirmPortalID,
		titleKey:    i18n.KeyConfirmReplaceNode,
		confirmKey:  i18n.KeyCapOverwrite,
		destructive: true,
		onConfirm:   g.ConfirmPasteOverwrite,
		onCancel:    g.CancelPasteOverwrite,
	})
	g.drum.portal().Open(g.pasteConfirmDlg.portalEntry(g.gridPaneScreenRect))
}

func (g *Game) closePasteConfirmPortal() {
	if g.drum == nil || g.drum.portal() == nil {
		return
	}
	g.drum.portal().Close(pasteConfirmPortalID)
}
