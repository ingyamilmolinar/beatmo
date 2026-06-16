package ui

import "github.com/hajimehoshi/ebiten/v2"

// gridLayer is a thin draw-only adapter: id + z + optional visibility +
// clip flag + a draw func bound to *Game. clip=true layers receive the
// grid-pane-bounds-clipped subimage (matching the original drawGridPane's
// `top`); clip=false layers draw to the unclipped screen (sidebar, cursor
// label, and node-glow-to-screen which the draw method handles internally).
type gridLayer struct {
	id   string
	z    int
	vis  func() bool
	clip bool
	draw func(dst *ebiten.Image)
}

func (l gridLayer) ID() string         { return l.id }
func (l gridLayer) ZIndex() int        { return l.z }
func (l gridLayer) ClipToBounds() bool { return l.clip }
func (l gridLayer) Visible() bool {
	if l.vis == nil {
		return true
	}
	return l.vis()
}
func (l gridLayer) Draw(dst *ebiten.Image) { l.draw(dst) }

// registerGridTree wires the grid pane's draw-only Layers into g.gridTree
// in ascending z-order. clip flags mirror the original drawGridPane targets:
// content → clipped `top`; sidebar + cursor label → unclipped screen. Grid
// input is NOT tree-routed (see GridTree doc comment); these are draw-only.
func (g *Game) registerGridTree() {
	g.gridTree.RegisterLayer(gridLayer{id: "grid-bg", z: GZBackground, clip: true, draw: g.drawGridBackground})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-edges", z: GZEdges, clip: true, draw: g.drawGridEdges})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-nodes", z: GZCanvas, clip: true, draw: g.drawGridNodes})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-pulses", z: GZPulses, clip: true, draw: g.drawGridPulses})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-coord-badge", z: GZCoordBadge, clip: true, draw: g.drawGridCoordBadge})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-move-mode", z: GZMoveMode, clip: true, vis: func() bool { return g.moveMode && g.movingNode != nil }, draw: g.drawGridMoveMode})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-connect-mode", z: GZConnectMode, clip: true, draw: g.drawConnectMode})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-longpress", z: GZLongPress, clip: true, vis: func() bool { return g.longPressPopup }, draw: g.drawLongPressPopup})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-move-confirm", z: GZMoveConfirm, clip: true, vis: func() bool { return g.moveConfirm }, draw: g.drawGridMoveConfirm})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-sidebar", z: GZSidebar, clip: false, vis: func() bool { return g.sidebar.IsOpen() }, draw: func(dst *ebiten.Image) { g.sidebar.Draw(dst) }})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-cursor-label", z: GZCursorLabel, clip: false, draw: g.drawGridCursorLabel})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-help-button", z: GZGridHelpButton, clip: true, vis: func() bool { return !Profile().IsMobile() }, draw: g.drawGridHelpButton})
}
