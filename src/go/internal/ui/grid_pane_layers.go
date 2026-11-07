package ui

import "github.com/hajimehoshi/ebiten/v2"

// clip=true gets the clipped subimage; clip=false the unclipped screen.
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

// Draw only: grid input is not tree-routed.
func (g *Game) registerGridTree() {
	g.gridTree.RegisterLayer(gridLayer{id: "grid-bg", z: GZBackground, clip: true, draw: g.drawGridBackground})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-edges", z: GZEdges, clip: true, draw: g.drawGridEdges})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-nodes", z: GZCanvas, clip: true, draw: g.drawGridNodes})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-pulses", z: GZPulses, clip: true, draw: g.drawGridPulses})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-guide", z: GZGuide, clip: true, vis: g.guideLayerVisible, draw: g.drawGridGuide})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-coord-badge", z: GZCoordBadge, clip: true, draw: g.drawGridCoordBadge})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-group-badges", z: GZGroupBadges, clip: true, vis: func() bool { return len(g.graph.Groups) > 0 }, draw: g.drawGridGroupBadges})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-move-mode", z: GZMoveMode, clip: true, vis: func() bool { return g.modes.Is(GridModeMove) || g.modes.Is(GridModeCopy) }, draw: g.drawGridMoveMode})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-connect-mode", z: GZConnectMode, clip: true, draw: g.drawConnectMode})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-mode-banner", z: GZModeBanner, clip: true, draw: g.drawGridModeBanner})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-marquee", z: GZMarquee, clip: true, vis: func() bool { return g.marquee.active }, draw: g.drawGridMarquee})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-longpress", z: GZLongPress, clip: true, vis: func() bool { return g.longPressPopup }, draw: g.drawLongPressPopup})
	// Clipped to the grid pane, so it can never render into the drum view; layout() also clamps it.
	g.gridTree.RegisterLayer(gridLayer{id: "grid-sidebar", z: GZSidebar, clip: true, vis: func() bool { return g.sidebar.IsOpen() }, draw: func(dst *ebiten.Image) { g.sidebar.Draw(dst) }})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-group-menu", z: GZGroupMenu, clip: false, vis: func() bool { return g.groupMenu.IsOpen() }, draw: func(dst *ebiten.Image) { g.groupMenu.Draw(dst) }})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-cursor-label", z: GZCursorLabel, clip: false, draw: g.drawGridCursorLabel})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-help-button", z: GZGridHelpButton, clip: true, draw: g.drawGridHelpButton})
	g.gridTree.RegisterLayer(gridLayer{id: "grid-toast", z: GZToast, clip: true, draw: g.drawGridToast})
}
