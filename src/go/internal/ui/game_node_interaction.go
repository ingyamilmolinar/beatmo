package ui

import (
	"math"
	"strings"

	"github.com/ingyamilmolinar/tunkul/core/model"
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
	// Compute in screen space so nodes grow when zooming out and shrink when
	// zooming in, providing strong visibility at bird's‑eye views.
	scale := g.cam.Scale
	baseScr := 12.0 / math.Max(scale, 0.001) // inverse with zoom
	// Also incorporate grid scale a bit so at extreme zoom-in we retain shape.
	baseScr = math.Max(baseScr, g.grid.UnitPixels(scale)*0.2)
	// Apply trigger animation toward a higher cap; do not clamp by neighbors
	// so nodes can collide visually when zoomed far out, per UI policy.
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
	// Enforce a minimum on-screen size for easier clicking: at least 3x the
	// current smallest baseline node, unless that would overlap neighbors.
	// Minimum on-screen radius when zoomed in to keep nodes clickable,
	// without blowing them up: ~10px radius (20px diameter).
	minFloor := 10.0
	if scale >= 1.0 && rScr < minFloor && n != nil {
		// Compute screen-space center of this node using baseline rect.
		x1, y1, x2, y2 := g.nodeScreenRect(n)
		cx := (x1 + x2) * 0.5
		cy := (y1 + y2) * 0.5
		// Check against other visible nodes using a conservative neighbor size.
		overlap := false
		for _, m := range g.nodes {
			if m == n {
				continue
			}
			mn, ok := g.graph.Nodes[m.ID]
			if !ok || mn.Type == model.NodeTypeInvisible {
				continue
			}
			mx1, my1, mx2, my2 := g.nodeScreenRect(m)
			mcx := (mx1 + mx2) * 0.5
			mcy := (my1 + my2) * 0.5
			// Use a conservative neighbor radius: cap at 16px baseline plus padding.
			neighScr := g.grid.NodeRadius(scale) * scale
			if neighScr < 16 {
				neighScr = 16
			}
			dx := cx - mcx
			dy := cy - mcy
			dist := math.Hypot(dx, dy)
			if dist < (minFloor + neighScr + 2) {
				overlap = true
				break
			}
		}
		if !overlap {
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
	// Mute nodes no longer enforce a sustained gate; returning zero limits
	// suppression to the scheduling index itself.
	return 0
}

func shouldGateMuteNode(n model.Node) bool {
	if n.Type != model.NodeTypeMute {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(n.Params.LogicKind))
	return kind != "" && kind != "none"
}
