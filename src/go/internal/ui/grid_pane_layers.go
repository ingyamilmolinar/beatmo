package ui

// registerGridTree wires the grid pane's Layers and Zones into g.gridTree
// in ascending z-order. Each participant is a thin adapter delegating to a
// (*Game).drawXxx method (see grid_pane_draw.go). Order here is cosmetic —
// GridTree sorts by ZIndex — but kept ascending for readability and to
// mirror the z-order table test.
func (g *Game) registerGridTree() {
	// Participants registered in Phase 2 tasks 6+.
}
