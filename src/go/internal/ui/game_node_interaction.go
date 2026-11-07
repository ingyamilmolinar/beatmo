package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// nodeAtScreen returns the topmost visible node whose on-screen rect contains (x,y).
func (g *Game) nodeAtScreen(x, y int) *uiNode {
	var best *uiNode
	bestDist := 1e12
	for _, n := range g.nodes {
		mn, ok := g.graph.Nodes[n.ID]
		if !ok || mn.Type == model.NodeTypeInvisible {
			continue
		}
		x1, y1, x2, y2 := g.nodeScreenRect(n)
		// Use visual radius for hit testing to match what the user sees.
		cx := (x1 + x2) * 0.5
		cy := (y1 + y2) * 0.5
		rv := g.nodeRadius(n) * g.cam.Scale
		hx1, hy1, hx2, hy2 := cx-rv, cy-rv, cx+rv, cy+rv
		if float64(x) >= hx1 && float64(x) <= hx2 && float64(y) >= hy1 && float64(y) <= hy2 {
			dx := float64(x) - cx
			dy := float64(y) - cy
			d := dx*dx + dy*dy
			if d < bestDist {
				bestDist = d
				best = n
			}
		}
	}
	return best
}

func (g *Game) nodeRadius(n *uiNode) float64 {
	// Screen space, so nodes grow when zoomed out and stay visible at a bird's-eye view.
	scale := g.cam.Scale
	baseScr := 12.0 / math.Max(scale, 0.001) // inverse with zoom
	// Also incorporate grid scale a bit so at extreme zoom-in we retain shape.
	baseScr = math.Max(baseScr, g.grid.UnitPixels(scale)*0.2)
	// No clamp by neighbours: nodes may visually collide when zoomed far out.
	capScr := 96.0 // generous upper bound
	rScr := baseScr
	if n != nil {
		if a := g.nodeAnimGet(n.ID); a > 0 {
			// Snappy ease-out cubic and restrained peak (~one-third)
			e := 1 - (1-a)*(1-a)*(1-a)
			peak := 0.35
			rScr = baseScr + (capScr-baseScr)*peak*e
		}
	}
	// A ~10 px radius floor keeps nodes clickable without blowing them up.
	minFloor := 10.0
	if scale >= 1.0 && rScr < minFloor && n != nil {
		// O(1): nodes sit on a grid with a known minimum spacing.
		unitScreen := g.grid.UnitPixels(scale) * scale
		if unitScreen >= (minFloor*2 + 2) {
			rScr = minFloor
		} else if len(g.nodes) <= 1 {
			// Single node: no neighbors, floor always safe.
			rScr = minFloor
		}
	}
	// Apply hover enlargement after enforcing minimums so hovered nodes remain larger.
	if g.hover == n {
		rScr *= 1.2
	}
	// Keep within absolute sane limits
	if rScr < 2 {
		rScr = 2
	} else if rScr > capScr {
		rScr = capScr
	}
	return rScr / scale
}

// Shared by a direct node click and a release on a guide snapped to one.
func (g *Game) selectNodeOpenSidebar(n *uiNode) {
	if g.sel != nil {
		g.sel.Selected = false
	}
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.computeSelNeighbors()
	g.coordBadgeNode = n
	g.coordBadgeFrame = g.frame
}

// computeSelNeighbors refreshes the neighbor set for the currently selected node.
func (g *Game) computeSelNeighbors() {
	g.selNeighbors = map[*uiNode]bool{}
	if g.sel == nil {
		return
	}
	for i := range g.edges {
		if g.edges[i].A == g.sel {
			g.selNeighbors[g.edges[i].B] = true
		} else if g.edges[i].B == g.sel {
			g.selNeighbors[g.edges[i].A] = true
		}
	}
}

func (g *Game) ensureGateSlices() {
	rows := 0
	if g.drum != nil {
		rows = len(g.drum.Rows)
	}
	if len(g.muteUntilByRow) != rows {
		old := g.muteUntilByRow
		g.muteUntilByRow = make([]int, rows)
		copy(g.muteUntilByRow, old)
	}
}

func (g *Game) muteHoldSteps(row, idx int, info model.BeatInfo) int {
	// Mute nodes gate only their scheduling index, not a sustained window.
	return 0
}
