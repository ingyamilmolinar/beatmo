package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// enterConnectMode activates connect mode: the next grid tap will create
// a directed edge from the given node to the tapped target.
func (g *Game) enterConnectMode(node *uiNode) {
	g.modes.Enter(GridModeConnect, node)
	// Visually select the source node.
	if g.sel != nil {
		g.sel.Selected = false
	}
	g.sel = node
	node.Selected = true
	g.computeSelNeighbors()
}

// handleConnectModeTap processes a grid tap while connect mode is active.
func (g *Game) handleConnectModeTap(x, y int) {
	from := g.modes.Subject()
	target := g.nodeAtScreen(x, y)
	if target == nil {
		// Tap on empty grid cancels connect mode.
		g.modes.Cancel("invalid-target")
		return
	}
	// Ignore self-tap.
	if target.ID == from.ID {
		return
	}
	// Reject invisible nodes.
	if mn, ok := g.graph.GetNodeByID(target.ID); ok && mn.Type == model.NodeTypeInvisible {
		if g.drum != nil {
			g.drum.notifyErrorKey(i18n.KeyNotifCannotConnectInvisible)
		}
		return
	}
	// Check perpendicularity: must share I or J coordinate.
	if from.I != target.I && from.J != target.J {
		if g.drum != nil {
			g.drum.notifyErrorKey(i18n.KeyNotifOnlyPerpendicular)
		}
		return
	}
	// Create edge (addEdge handles duplicate check internally).
	g.addEdge(from, target)
	g.modes.Commit()
}

// drawConnectMode renders the connect mode visual feedback: a cyan outline
// around the source node. The armed-mode banner is drawn once for all three
// modes (move/copy/connect) by drawGridModeBanner (Task 9) — this function no
// longer draws a banner of its own.
func (g *Game) drawConnectMode(dst *ebiten.Image) {
	if !g.modes.Is(GridModeConnect) {
		return
	}
	from := g.modes.Subject()
	// Cyan outline around source node.
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(from)
	hlCol := WithAlpha(TokenAccent(), 255)
	var id ebiten.GeoM
	DrawLineCam(dst, sx1-1, sy1-1, sx2+1, sy1-1, &id, hlCol, 2)
	DrawLineCam(dst, sx2+1, sy1-1, sx2+1, sy2+1, &id, hlCol, 2)
	DrawLineCam(dst, sx2+1, sy2+1, sx1-1, sy2+1, &id, hlCol, 2)
	DrawLineCam(dst, sx1-1, sy2+1, sx1-1, sy1-1, &id, hlCol, 2)
}
