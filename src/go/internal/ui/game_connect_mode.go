package ui

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// enterConnectMode activates connect mode: the next grid tap will create
// a directed edge from the given node to the tapped target.
func (g *Game) enterConnectMode(node *uiNode) {
	g.connectMode = true
	g.connectFromNode = node
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
	target := g.nodeAtScreen(x, y)
	if target == nil {
		// Tap on empty grid cancels connect mode.
		g.cancelConnectMode()
		return
	}
	// Ignore self-tap.
	if target.ID == g.connectFromNode.ID {
		return
	}
	// Reject invisible nodes.
	if mn, ok := g.graph.GetNodeByID(target.ID); ok && mn.Type == model.NodeTypeInvisible {
		if g.drum != nil {
			g.drum.notifyError(i18n.T(i18n.KeyNotifCannotConnectInvisible))
		}
		return
	}
	// Check perpendicularity: must share I or J coordinate.
	from := g.connectFromNode
	if from.I != target.I && from.J != target.J {
		if g.drum != nil {
			g.drum.notifyError(i18n.T(i18n.KeyNotifOnlyPerpendicular))
		}
		return
	}
	// Create edge (addEdge handles duplicate check internally).
	g.addEdge(from, target)
	g.cancelConnectMode()
}

// cancelConnectMode clears connect mode state.
func (g *Game) cancelConnectMode() {
	g.connectMode = false
	g.connectFromNode = nil
}

// drawConnectMode renders the connect mode visual feedback: a banner at the
// top of the grid pane and a cyan outline around the source node.
func (g *Game) drawConnectMode(dst *ebiten.Image) {
	if !g.connectMode || g.connectFromNode == nil {
		return
	}
	// Banner at top of grid pane.
	bannerText := fmt.Sprintf("Connect from (%d,%d) — tap target node", g.connectFromNode.I, g.connectFromNode.J)
	bannerW := TextWidth(bannerText) + 16
	bannerX := (g.split.GridW(g.winW) - bannerW) / 2
	if bannerX < 0 {
		bannerX = 0
	}
	bannerRect := image.Rect(bannerX, 2, bannerX+bannerW, 22)
	drawRoundedRect(dst, bannerRect, colPanelBG, popupCornerRadius(), true)
	drawRoundedRect(dst, bannerRect, colPanelBorder, popupCornerRadius(), false)
	DrawTextAt(dst, bannerText, bannerX+8, 5)

	// Cyan outline around source node.
	sx1, sy1, sx2, sy2 := g.nodeScreenRect(g.connectFromNode)
	hlCol := WithAlpha(TokenAccent(), 255)
	var id ebiten.GeoM
	DrawLineCam(dst, sx1-1, sy1-1, sx2+1, sy1-1, &id, hlCol, 2)
	DrawLineCam(dst, sx2+1, sy1-1, sx2+1, sy2+1, &id, hlCol, 2)
	DrawLineCam(dst, sx2+1, sy2+1, sx1-1, sy2+1, &id, hlCol, 2)
	DrawLineCam(dst, sx1-1, sy2+1, sx1-1, sy1-1, &id, hlCol, 2)
}
