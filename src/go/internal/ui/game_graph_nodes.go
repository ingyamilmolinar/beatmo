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
	// One atomic undo step: a click that auto-stitches a node into an existing
	// edge emits an edge-delete + two edge-adds + the node-add; they must
	// collapse into a single "add node" step.
	beginUndoGroup("add node")
	defer endUndoGroup()
	// Remember whether we're in origin-selection mode for this placement so
	// we can avoid side effects (like auto-stitching other circuits).
	selectingOrigin := (g.pendingStartRow >= 0)
	importing := g.importing
	if selectingOrigin {
		g.logger.Debugf("[origin] placing node at grid=(%d,%d) nodeType=%v pendingRow=%d", i, j, nodeType, g.pendingStartRow)
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
					g.logger.Debugf("[origin] set origin row=%d to existing node id=%d grid=(%d,%d)", row, n.ID, n.I, n.J)
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
		g.logger.Debugf("[game] node created id=%d grid=(%d,%d)", n.ID, i, j)
	case model.NodeTypeSilent:
		g.logger.Debugf("[game] silent node created id=%d grid=(%d,%d)", n.ID, i, j)
	case model.NodeTypeMute:
		g.logger.Debugf("[game] mute node created id=%d grid=(%d,%d)", n.ID, i, j)
	default:
		g.logger.Debugf("[game] invisible node created id=%d grid=(%d,%d)", n.ID, i, j)
	}
	// Select newly created regular nodes to match test expectations.
	if nodeType == model.NodeTypeRegular && !importing {
		if g.sel != nil {
			g.sel.Selected = false
		}
		g.sel = n
		n.Selected = true
		g.computeSelNeighbors()
		g.coordBadgeNode = n
		g.coordBadgeFrame = g.frame
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
		g.logger.Debugf("[origin] created node id=%d grid=(%d,%d) for row=%d (no stitch)", n.ID, n.I, n.J, g.pendingStartRow)
	}
	if !importing {
		g.updateBeatInfos()
	}
	emitNodeAdded(n, nodeType)
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
	// One atomic undo step for the whole delete: node removal, dropped edges,
	// cascaded row deletion, and neighbour reconnection all collapse into a
	// single step (see UndoManager.beginGroup). Without this, deleting a
	// mid-circuit node recorded several steps and a single Ctrl+Z left a
	// partial graph (a node stranded in a different position).
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
	g.graph.RemoveNode(n.ID)
	g.removeNodeCache(n.ID)
	g.notifyPredictorNode(n.ID)
	delete(g.nodesByID, n.ID)
	g.logger.Debugf("[game] node deleted id=%d grid=(%d,%d)", n.ID, n.I, n.J)
	emitNodeDeleted(n.ID, n.I, n.J)

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

// pruneDanglingNodeRefs ties transient node-scoped UI to the lifecycle of the
// nodes themselves: any reference to a node that is no longer in the graph is
// repointed to the live node with the same id, or dropped — and an open node
// menu (sidebar) for a vanished node is closed.
//
// Per-node delete hooks (deleteNodeInternal) clear these refs by pointer, but a
// graph REBUILD (Import, which the undo/redo restore rides) replaces every node
// wholesale without running those hooks, so a deleted/undone node would
// otherwise leave its coordinate badge drawing and its menu open against a node
// that no longer exists. Called at the end of Import and once per frame, so the
// invariant holds regardless of HOW a node left the graph.
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
}

// moveNodeEdgeLoss previews how many edges would be lost if the node moved
// to (newI, newJ). An edge is kept only if it remains orthogonal (shares I or J)
// with the other endpoint at the new position.
func (g *Game) moveNodeEdgeLoss(n *uiNode, newI, newJ int) int {
	loss := 0
	for _, e := range g.edges {
		if e.A.ID != n.ID && e.B.ID != n.ID {
			continue
		}
		other := e.B
		if e.B.ID == n.ID {
			other = e.A
		}
		if other.I != newI && other.J != newJ {
			loss++
		}
	}
	return loss
}

// moveNode moves a node to a new grid position, preserving its NodeID.
// Returns false if the destination is occupied.
func (g *Game) moveNode(n *uiNode, newI, newJ int) bool {
	// Check destination is empty
	if existing := g.nodeAt(newI, newJ); existing != nil && existing.ID != n.ID {
		return false
	}

	// One atomic undo step: the edge teardown, the move, and the edge re-add all
	// collapse into a single "move node" step.
	beginUndoGroup("move node")
	defer endUndoGroup()

	// Collect edges touching this node (preserve direction)
	type edgeRef struct {
		from, to *uiNode
	}
	var touching []edgeRef
	for _, e := range g.edges {
		if e.A.ID == n.ID {
			touching = append(touching, edgeRef{e.A, e.B})
		} else if e.B.ID == n.ID {
			touching = append(touching, edgeRef{e.A, e.B})
		}
	}

	// Remove all touching edges
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

	// Re-add edges that remain orthogonal at the new position
	for _, er := range touching {
		from, to := er.from, er.to
		if from.I == to.I || from.J == to.J {
			g.addEdgeNoRefresh(from, to)
		}
	}

	g.notifyPredictorNode(n.ID)
	g.edgesDirty = true
	g.updateBeatInfos()
	g.computeSelNeighbors()

	// Set coordinate badge for the moved node
	g.coordBadgeNode = n
	g.coordBadgeFrame = g.frame

	emitNodeMoved(n.ID, newI, newJ)
	return true
}

// cancelMoveMode clears all move mode state.
func (g *Game) cancelMoveMode() {
	g.moveMode = false
	g.movingNode = nil
	g.moveConfirm = false
	g.moveConfirmI = 0
	g.moveConfirmJ = 0
	g.moveEdgeLoss = 0
	g.moveSkipRelease = false
}
