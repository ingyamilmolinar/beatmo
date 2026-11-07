package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
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
	// One undo step: an auto-stitch emits an edge delete, two edge adds and the node add.
	beginUndoGroup("add node")
	defer endUndoGroup()
	// Origin selection must not auto-stitch other circuits.
	selectingOrigin := (g.pendingStartRow >= 0)
	importing := g.importing
	if selectingOrigin {
		g.logger.Debugf("[origin] placing node at grid=(%d,%d) nodeType=%v pendingRow=%d", i, j, nodeType, g.pendingStartRow)
	}
	if n := g.nodeAt(i, j); n != nil {
		// Upgrade an invisible node here rather than blocking the placement.
		if nodeType.Fires() {
			if g.pendingStartRow >= 0 {
				row := g.pendingStartRow
				if other, ok := g.nodeRows[n.ID]; ok && other != row {
					// Only disallow if other circuit is currently audible during playback.
					if g.Playing() && g.rowIsAudible(other) {
						// Keep the pending selection so a click inside the intended circuit does not toggle it off.
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
					g.logger.Debugf("[origin] set origin row=%d to existing node id=%d grid=(%d,%d)", row, n.ID, n.I, n.J)
					if !importing {
						g.updateBeatInfos()
					}
				}
				// Suppress transient visuals briefly so no pulse crosses circuits.
				g.pendingStartRow = -1
				g.quietFrames = 2
			} else if node, ok := g.graph.GetNodeByID(n.ID); ok && node.Type == model.NodeTypeInvisible {
				node.Type = model.NodeTypeRegular
				g.graph.Nodes[n.ID] = node
				g.cacheNode(n.ID)
				if !importing {
					g.notifyPredictorNode(n.ID)
				}
				g.logger.Debugf("[game] Upgraded invisible node to regular at grid=(%d,%d)", i, j)
				g.logger.Debugf("[game] node upgraded to regular id=%d grid=(%d,%d)", n.ID, i, j)
				emitNodeTypeChanged(n.ID, model.NodeTypeInvisible, model.NodeTypeRegular)
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

	if nodeType.Fires() {
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
			// Suppress transient visuals briefly so no pulse crosses circuits.
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
	case model.NodeTypeRegular, model.NodeTypeChord:
		g.logger.Debugf("[game] node created id=%d grid=(%d,%d)", n.ID, i, j)
	case model.NodeTypeSilent:
		g.logger.Debugf("[game] silent node created id=%d grid=(%d,%d)", n.ID, i, j)
	case model.NodeTypeMute:
		g.logger.Debugf("[game] mute node created id=%d grid=(%d,%d)", n.ID, i, j)
	default:
		g.logger.Debugf("[game] invisible node created id=%d grid=(%d,%d)", n.ID, i, j)
	}
	// Select newly created regular nodes to match test expectations.
	if nodeType.Fires() && !importing {
		if g.sel != nil {
			g.sel.Selected = false
		}
		g.sel = n
		n.Selected = true
		g.computeSelNeighbors()
		g.coordBadgeNode = n
		g.coordBadgeFrame = g.frame
	}
	// Origin selection never alters other circuits.
	if !selectingOrigin && !importing {
		// A node on an existing edge splits it, so inserting into a circuit is one click.
		g.stitchEdgesAt(n)
	}
	if selectingOrigin {
		g.logger.Debugf("[origin] created node id=%d grid=(%d,%d) for row=%d (no stitch)", n.ID, n.I, n.J, g.pendingStartRow)
	}
	if !importing {
		g.updateBeatInfos()
	}
	emitNodeAdded(n, nodeType)
	return n
}

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
		if a.I == b.I && a.I == n.I { //nolint:gocritic // badCond: intentional three-way equality check
			// Vertical: check J strictly between endpoints.
			minJ, maxJ := a.J, b.J
			if minJ > maxJ {
				minJ, maxJ = maxJ, minJ
			}
			if n.J > minJ && n.J < maxJ {
				toSplit = append(toSplit, pair{a, b})
			}
		} else if a.J == b.J && a.J == n.J { //nolint:gocritic // badCond: intentional three-way equality check
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
	// One undo step for the whole delete, or one Ctrl+Z leaves a partial graph.
	beginUndoGroup("delete node")
	defer endUndoGroup()
	// Capture predecessors and successors before removal for potential reconnection.
	preds := []*uiNode{}
	succs := []*uiNode{}
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
	removed, _ := g.graph.GetNodeByID(n.ID)
	g.graph.RemoveNode(n.ID)
	g.removeNodeCache(n.ID)
	g.notifyPredictorNode(n.ID)
	delete(g.nodesByID, n.ID)
	g.logger.Debugf("[game] node deleted id=%d grid=(%d,%d)", n.ID, n.I, n.J)
	emitNodeDeleted(n.ID, n.I, n.J, removed.Type)

	// If this node was the global start, clear it.
	if g.graph.StartNodeID == n.ID {
		g.graph.StartNodeID = model.InvalidNodeID
	}
	// In reverse, so indices stay stable.
	toDel := []int{}
	for i := range g.drum.Rows {
		if g.drum.Rows[i].Origin == n.ID {
			toDel = append(toDel, i)
		}
	}
	for i := len(toDel) - 1; i >= 0; i-- {
		g.drum.DeleteRow(toDel[i])
	}

	// Reconnect aligned neighbours so straight paths survive a deleted middle node.
	for _, p := range preds {
		for _, s := range succs {
			if p == nil || s == nil || p.ID == s.ID {
				continue
			}
			if p.I != s.I && p.J != s.J {
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
	if g.coordBadgeNode == n {
		g.coordBadgeNode = nil
	}
	if g.sidebar != nil && g.sidebar.Node() == n {
		g.sidebar.Close()
	}
	if g.start == n {
		g.start = nil
	}
	if updateBeatInfos {
		g.updateBeatInfos()
	}
}

// A rebuild (Import, and so undo/redo) replaces every node without the per-node delete hooks, so
// this repoints or drops node-scoped UI refs. Runs per frame, whatever removed the node.
func (g *Game) pruneDanglingNodeRefs() {
	if g.coordBadgeNode != nil {
		if live, ok := g.nodesByID[g.coordBadgeNode.ID]; ok {
			g.coordBadgeNode = live
		} else {
			g.coordBadgeNode = nil
		}
	}
	if g.sidebar != nil && g.sidebar.IsOpen() {
		n := g.sidebar.Node()
		if n == nil {
			g.sidebar.Close()
		} else if _, ok := g.nodesByID[n.ID]; !ok {
			g.sidebar.Close()
		}
	}
	if subject := g.modes.Subject(); subject != nil {
		if _, ok := g.nodesByID[subject.ID]; !ok {
			g.CancelBreakCircuit()
			g.modes.Cancel("subject-deleted")
			g.moveSkipRelease = false
		}
	}
}

// Keeps its NodeID; false if the destination is occupied.
func (g *Game) moveNode(n *uiNode, newI, newJ int) bool {
	// Check destination is empty
	if existing := g.nodeAt(newI, newJ); existing != nil && existing.ID != n.ID {
		return false
	}

	beginUndoGroup("move node")
	defer endUndoGroup()

	// Classify before touching edges; n reconnects only through the destination splice.
	plan := g.moveHealPlan(n)
	type edgeRef struct{ from, to *uiNode }
	var touching []edgeRef
	for _, e := range g.edges {
		if e.A.ID == n.ID || e.B.ID == n.ID {
			touching = append(touching, edgeRef{e.A, e.B})
		}
	}
	for _, er := range touching {
		g.deleteEdgeNoRefresh(er.from, er.to)
	}

	// Move the node in the graph model
	g.graph.MoveNode(n.ID, newI, newJ)
	g.cacheNode(n.ID)

	// Update uiNode coordinates
	n.I = newI
	n.J = newJ
	unit := g.grid.Unit()
	n.X = float64(newI) * unit
	n.Y = float64(newJ) * unit

	// After the coords update, so n's old cell no longer splits the heal (moving along its own chain
	// re-splices it).
	if plan.class == healPassThrough {
		if g.addEdgeNoRefresh(plan.from, plan.to) {
			for _, m := range g.nodesOnSpan(plan.from, plan.to) {
				g.stitchEdgesAt(m)
			}
		}
	}

	// Same as add/paste: a node dropped on an edge joins the path there; its own edges never split.
	if !g.importing {
		g.stitchEdgesAt(n)
	}

	g.notifyPredictorNode(n.ID)
	g.edgesDirty = true
	g.updateBeatInfos()
	g.computeSelNeighbors()

	// Set coordinate badge for the moved node
	g.coordBadgeNode = n
	g.coordBadgeFrame = g.frame

	moved, _ := g.graph.GetNodeByID(n.ID)
	emitNodeMoved(n.ID, newI, newJ, moved.Type)
	return true
}
