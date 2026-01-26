package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
)

/* ─────────────────────── graph helpers ─────────────────────── */

func (g *Game) nodeAt(i, j int) *uiNode {
	for _, n := range g.nodes {
		if n.I == i && n.J == j {
			return n
		}
	}
	return nil
}

func (g *Game) nodeByID(id model.NodeID) *uiNode {
	if n := g.nodesByID[id]; n != nil {
		return n
	}
	return nil
}

func (g *Game) tryAddNode(i, j int, nodeType model.NodeType) *uiNode {
	// Remember whether we're in origin-selection mode for this placement so
	// we can avoid side effects (like auto-stitching other circuits).
	selectingOrigin := (g.pendingStartRow >= 0)
	importing := g.importing
	if selectingOrigin {
		g.logger.Debugf("[ORIGIN] placing node at grid=(%d,%d) nodeType=%v pendingRow=%d", i, j, nodeType, g.pendingStartRow)
	}
	if n := g.nodeAt(i, j); n != nil {
		// If there's an invisible node here and we want a regular node,
		// upgrade the existing node rather than blocking the placement.
		if nodeType == model.NodeTypeRegular {
			if g.pendingStartRow >= 0 {
				row := g.pendingStartRow
				if other, ok := g.nodeRows[n.ID]; ok && other != row {
					// Only disallow if other circuit is currently audible during playback.
					if g.Playing() && g.rowIsAudible(other) {
						// Keep the pending selection active so the user can click
						// a node within the intended circuit without toggling off.
						return n
					}
				}
				if row >= 0 && row < len(g.drum.Rows) {
					if old := g.drum.Rows[row].Node; old != nil {
						old.Start = false
					}
					g.drum.Rows[row].Origin = n.ID
					g.drum.Rows[row].Node = n
					n.Start = true
					if row == 0 {
						g.start = n
						g.graph.StartNodeID = n.ID
					}
					g.logger.Debugf("[ORIGIN] set origin row=%d to existing node id=%d grid=(%d,%d)", row, n.ID, n.I, n.J)
					if !importing {
						g.updateBeatInfos()
					}
				}
				// Finalize origin selection and suppress transient visuals
				// for a couple of frames to avoid any cross-circuit pulses.
				g.pendingStartRow = -1
				g.quietFrames = 2
			} else if node, ok := g.graph.GetNodeByID(n.ID); ok && node.Type == model.NodeTypeInvisible {
				node.Type = model.NodeTypeRegular
				g.graph.Nodes[n.ID] = node
				g.cacheNode(n.ID)
				if !importing {
					g.notifyPredictorNode(n.ID)
				}
				g.logger.Debugf("[GAME] Upgraded invisible node to regular at grid=(%d,%d)", i, j)
				g.logger.Infof("[GAME] Node upgraded to regular id=%d grid=(%d,%d)", n.ID, i, j)
				if g.start == nil {
					g.start = n
					n.Start = true
					g.graph.StartNodeID = n.ID
				}
				if !importing {
					g.updateBeatInfos()
				}
			}
		}
		return n
	}
	id := g.graph.AddNode(i, j, nodeType)
	g.cacheNode(id)
	unit := g.grid.Unit()
	n := &uiNode{ID: id, I: i, J: j, X: float64(i) * unit, Y: float64(j) * unit}
	if !importing {
		g.notifyPredictorNode(id)
	}

	if nodeType == model.NodeTypeRegular {
		if g.pendingStartRow >= 0 {
			row := g.pendingStartRow
			if row >= 0 && row < len(g.drum.Rows) {
				g.drum.Rows[row].Origin = n.ID
				g.drum.Rows[row].Node = n
				n.Start = true
				if row == 0 {
					g.start = n
					g.graph.StartNodeID = n.ID
				}
			}
			// Finalize origin selection and suppress transient visuals
			// for a couple of frames to avoid any cross-circuit pulses.
			g.pendingStartRow = -1
			g.quietFrames = 2
		} else if g.start == nil {
			g.start = n
			n.Start = true
			g.graph.StartNodeID = n.ID
		}
	}
	g.nodes = append(g.nodes, n)
	g.nodesByID[n.ID] = n
	switch nodeType {
	case model.NodeTypeRegular:
		g.logger.Infof("[GAME] Node created id=%d grid=(%d,%d)", n.ID, i, j)
	case model.NodeTypeSilent:
		g.logger.Infof("[GAME] Silent node created id=%d grid=(%d,%d)", n.ID, i, j)
	case model.NodeTypeMute:
		g.logger.Infof("[GAME] Mute node created id=%d grid=(%d,%d)", n.ID, i, j)
	default:
		g.logger.Infof("[GAME] Invisible node created id=%d grid=(%d,%d)", n.ID, i, j)
	}
	// Select newly created regular nodes to match test expectations.
	if nodeType == model.NodeTypeRegular && !importing {
		if g.sel != nil {
			g.sel.Selected = false
		}
		g.sel = n
		n.Selected = true
		g.computeSelNeighbors()
	}
	// Auto-stitch only during normal editing. When placing a node as part of
	// origin selection, never alter other circuits.
	if !selectingOrigin && !importing {
		// Auto-stitch: if this new node lies along an existing edge (orthogonal),
		// split that edge into two edges that terminate at the new node. This
		// makes inserting nodes into existing circuits a one-click operation.
		g.stitchEdgesAt(n)
	}
	if selectingOrigin {
		g.logger.Debugf("[ORIGIN] created node id=%d grid=(%d,%d) for row=%d (no stitch)", n.ID, n.I, n.J, g.pendingStartRow)
	}
	if !importing {
		g.updateBeatInfos()
	}
	return n
}

// stitchEdgesAt finds UI edges that pass through the given node's grid
// coordinate and replaces each such edge A-B with A-n and n-B.
func (g *Game) stitchEdgesAt(n *uiNode) {
	if n == nil {
		return
	}
	// Collect candidate edges to avoid mutating g.edges while iterating it.
	type pair struct{ a, b *uiNode }
	var toSplit []pair
	for _, e := range g.edges {
		a, b := e.A, e.B
		// Only consider strictly orthogonal edges.
		if a.I == b.I && a.I == n.I {
			// Vertical: check J strictly between endpoints.
			minJ, maxJ := a.J, b.J
			if minJ > maxJ {
				minJ, maxJ = maxJ, minJ
			}
			if n.J > minJ && n.J < maxJ {
				toSplit = append(toSplit, pair{a, b})
			}
		} else if a.J == b.J && a.J == n.J {
			// Horizontal: check I strictly between endpoints.
			minI, maxI := a.I, b.I
			if minI > maxI {
				minI, maxI = maxI, minI
			}
			if n.I > minI && n.I < maxI {
				toSplit = append(toSplit, pair{a, b})
			}
		}
	}
	// Perform splits: delete old, create two new.
	changed := false
	for _, p := range toSplit {
		if g.deleteEdgeNoRefresh(p.a, p.b) {
			changed = true
		}
		if g.addEdgeNoRefresh(p.a, n) {
			changed = true
		}
		if g.addEdgeNoRefresh(n, p.b) {
			changed = true
		}
	}
	if changed && g.graph.StartNodeID != model.InvalidNodeID {
		if !g.importing {
			g.updateBeatInfos()
		}
	}
	if changed {
		if !g.importing {
			g.computeSelNeighbors()
		}
	}
}

func (g *Game) deleteNode(n *uiNode) {
	g.deleteNodeInternal(n, true)
}

func (g *Game) deleteNodeInternal(n *uiNode, updateBeatInfos bool) {
	// Capture predecessors and successors before removal for potential reconnection.
	preds := []*uiNode{}
	succs := []*uiNode{}
	row := -1
	if g.nodeRows != nil {
		if r, ok := g.nodeRows[n.ID]; ok {
			row = r
		}
	}
	for e := range g.graph.Edges {
		if e[1] == n.ID { // pred -> n
			if p := g.nodesByID[e[0]]; p != nil {
				preds = append(preds, p)
			}
		}
		if e[0] == n.ID { // n -> succ
			if s := g.nodesByID[e[1]]; s != nil {
				succs = append(succs, s)
			}
		}
	}

	/* remove from slice */
	for idx, v := range g.nodes {
		if v.ID == n.ID {
			g.nodes = append(g.nodes[:idx], g.nodes[idx+1:]...)
			break
		}
	}
	/* drop touching edges */
	out := g.edges[:0]
	for _, e := range g.edges {
		if e.A.ID != n.ID && e.B.ID != n.ID {
			out = append(out, e)
		}
	}
	g.edges = out
	g.edgesDirty = true

	// Remove from graph last so Edges map is still available above.
	g.graph.RemoveNode(n.ID)
	g.removeNodeCache(n.ID)
	g.notifyPredictorNode(n.ID)
	if row < 0 {
		if g.nodeRows != nil {
			if r, ok := g.nodeRows[n.ID]; ok {
				row = r
			}
		}
	}
	delete(g.nodesByID, n.ID)
	g.logger.Infof("[GAME] Node deleted id=%d grid=(%d,%d)", n.ID, n.I, n.J)

	// If this node was the global start, clear it.
	if g.graph.StartNodeID == n.ID {
		g.graph.StartNodeID = model.InvalidNodeID
	}
	// Delete any drum row that used this node as its origin to avoid confusing
	// stale references. Do this in reverse to keep indices stable.
	toDel := []int{}
	for i := range g.drum.Rows {
		if g.drum.Rows[i].Origin == n.ID {
			toDel = append(toDel, i)
		}
	}
	for i := len(toDel) - 1; i >= 0; i-- {
		g.drum.DeleteRow(toDel[i])
	}

	// Reconnect preds -> succs when aligned on the same row or column, to
	// preserve straight paths across deleted in-between nodes.
	for _, p := range preds {
		for _, s := range succs {
			if p == nil || s == nil || p.ID == s.ID {
				continue
			}
			if !(p.I == s.I || p.J == s.J) {
				continue
			} // only orthogonal
			// Avoid duplicate edges
			if _, ok := g.graph.Edges[[2]model.NodeID{p.ID, s.ID}]; ok {
				continue
			}
			g.addEdge(p, s)
		}
	}

	if g.sel == n {
		g.sel = nil
	}
	if g.start == n {
		g.start = nil
	}
	if updateBeatInfos {
		g.updateBeatInfos()
	}
}
