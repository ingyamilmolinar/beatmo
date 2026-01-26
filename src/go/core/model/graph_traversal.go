package model

import "sort"

func (g *Graph) CalculateBeatRow() ([]BeatInfo, bool, int) {
	g.logger.Debugf("[GRAPH] CalculateBeatRow: Start. StartNodeID: %d, BeatLengthValue: %d", g.StartNodeID, g.beatLengthValue)

	if g.StartNodeID == InvalidNodeID {
		beatRow := make([]BeatInfo, g.beatLengthValue)
		for i := range beatRow {
			beatRow[i] = BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: -1, J: -1}
		}
		g.logger.Debugf("[GRAPH] CalculateBeatRow: No start node, returning empty beat row: %v", beatRow)
		return beatRow, false, -1
	}

	beatRow := []BeatInfo{}
	// Build a coordinate index once to resolve any explicit nodes present at
	// intermediate coordinates without scanning for each step.
	byCoord := make(map[[2]int]NodeID, len(g.Nodes))
	for id, n := range g.Nodes {
		byCoord[[2]int{n.I, n.J}] = id
	}
	// Build adjacency once to avoid scanning all edges per step.
	nbrs := make(map[NodeID][]NodeID)
	for e := range g.Edges {
		a, b := e[0], e[1]
		nbrs[a] = append(nbrs[a], b)
	}
	visitedBeatIdx := make(map[NodeID]int)
	isLoop := false
	loopStartBeatIdx := -1

	cur := g.StartNodeID
	for cur != InvalidNodeID {
		if idx, ok := visitedBeatIdx[cur]; ok {
			isLoop = true
			loopStartBeatIdx = idx
			g.logger.Debugf("[GRAPH] CalculateBeatRow: Loop detected at node %d (beatIdx=%d)", cur, idx)
			break
		}
		nodeCur, ok := g.Nodes[cur]
		if !ok {
			g.logger.Warnf("[GRAPH] Missing node %d in Nodes; stopping traversal", cur)
			break
		}
		visitedBeatIdx[cur] = len(beatRow)
		beatRow = append(beatRow, BeatInfo{NodeID: cur, NodeType: nodeCur.Type, I: nodeCur.I, J: nodeCur.J})

		// Collect outgoing neighbors.
		neighbors := nbrs[cur]
		if len(neighbors) == 0 {
			break
		}
		// Deterministic order: by J then I (stable with prior behavior)
		sort.Slice(neighbors, func(i, j int) bool {
			a := g.Nodes[neighbors[i]]
			b := g.Nodes[neighbors[j]]
			if a.J != b.J {
				return a.J < b.J
			}
			return a.I < b.I
		})
		next := neighbors[0]
		nodeNext := g.Nodes[next]

		// Append intermediate steps between cur and next (exclusive).
		// Even if there is an explicit node at an intermediate coordinate,
		// treat it as an invisible pass-through to avoid duplicate triggers
		// when edges skip across existing nodes.
		if nodeCur.I == nodeNext.I && nodeCur.J == nodeNext.J {
			// Self-loop: no intermediate steps to synthesize.
		} else if nodeCur.I == nodeNext.I {
			step := 1
			if nodeCur.J > nodeNext.J {
				step = -1
			}
			for j := nodeCur.J + step; j != nodeNext.J; j += step {
				if id, ok := byCoord[[2]int{nodeCur.I, j}]; ok {
					n := g.Nodes[id]
					if n.Type == NodeTypeInvisible {
						beatRow = append(beatRow, BeatInfo{NodeID: id, NodeType: NodeTypeInvisible, I: n.I, J: n.J})
					} else {
						beatRow = append(beatRow, BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: n.I, J: n.J})
					}
				} else {
					beatRow = append(beatRow, BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: nodeCur.I, J: j})
				}
			}
		} else if nodeCur.J == nodeNext.J {
			step := 1
			if nodeCur.I > nodeNext.I {
				step = -1
			}
			for i := nodeCur.I + step; i != nodeNext.I; i += step {
				if id, ok := byCoord[[2]int{i, nodeCur.J}]; ok {
					n := g.Nodes[id]
					if n.Type == NodeTypeInvisible {
						beatRow = append(beatRow, BeatInfo{NodeID: id, NodeType: NodeTypeInvisible, I: n.I, J: n.J})
					} else {
						beatRow = append(beatRow, BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: n.I, J: n.J})
					}
				} else {
					beatRow = append(beatRow, BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: i, J: nodeCur.J})
				}
			}
		}
		cur = next
	}

	g.logger.Tracef("[GRAPH] CalculateBeatRow: Raw beatRow before padding/loop handling: %v", beatRow)

	if isLoop {
		prefix := beatRow[:loopStartBeatIdx]
		loopSegment := beatRow[loopStartBeatIdx:]
		final := make([]BeatInfo, 0, g.beatLengthValue)
		final = append(final, prefix...)
		for len(final) < g.beatLengthValue && len(loopSegment) > 0 {
			final = append(final, loopSegment...)
		}
		beatRow = final
		g.logger.Tracef("[GRAPH] CalculateBeatRow: BeatRow after loop expansion: %v", beatRow)
	}

	if len(beatRow) > g.beatLengthValue {
		beatRow = beatRow[:g.beatLengthValue]
		g.logger.Tracef("[GRAPH] CalculateBeatRow: Trimmed beatRow to length %d: %v", g.beatLengthValue, beatRow)
	} else {
		for len(beatRow) < g.beatLengthValue {
			beatRow = append(beatRow, BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: -1, J: -1})
		}
		g.logger.Tracef("[GRAPH] CalculateBeatRow: Padded beatRow to length %d: %v", g.beatLengthValue, beatRow)
	}

	g.logger.Debugf("[GRAPH] CalculateBeatRow: End. Final beatRow length: %d, IsLoop: %t", len(beatRow), isLoop)
	return beatRow, isLoop, loopStartBeatIdx
}

// CalculateBeatRowUnbounded returns the raw traversal path starting at
// StartNodeID, including synthesized intermediate invisible BeatInfos between
// orthogonal endpoints. The result is not trimmed or padded to BeatLength and
// does not expand loops; callers can apply their own sizing rules.
func (g *Graph) CalculateBeatRowUnbounded() ([]BeatInfo, bool, int) {
	if g.StartNodeID == InvalidNodeID {
		return nil, false, -1
	}
	// Build coord index once for intermediate lookups.
	byCoord := make(map[[2]int]NodeID, len(g.Nodes))
	for id, n := range g.Nodes {
		byCoord[[2]int{n.I, n.J}] = id
	}
	// Build adjacency once per call.
	nbrsBy := make(map[NodeID][]NodeID)
	for e := range g.Edges {
		a, b := e[0], e[1]
		nbrsBy[a] = append(nbrsBy[a], b)
	}
	var beatRow []BeatInfo
	visited := make(map[NodeID]int)
	isLoop := false
	loopStart := -1
	cur := g.StartNodeID
	for cur != InvalidNodeID {
		if idx, ok := visited[cur]; ok {
			isLoop = true
			loopStart = idx
			break
		}
		nodeCur, ok := g.Nodes[cur]
		if !ok {
			break
		}
		visited[cur] = len(beatRow)
		beatRow = append(beatRow, BeatInfo{NodeID: cur, NodeType: nodeCur.Type, I: nodeCur.I, J: nodeCur.J})
		// Neighbors
		nbrs := nbrsBy[cur]
		if len(nbrs) == 0 {
			break
		}
		sort.Slice(nbrs, func(i, j int) bool {
			a := g.Nodes[nbrs[i]]
			b := g.Nodes[nbrs[j]]
			if a.J != b.J {
				return a.J < b.J
			}
			return a.I < b.I
		})
		next := nbrs[0]
		nodeNext := g.Nodes[next]
		if nodeCur.I == nodeNext.I && nodeCur.J == nodeNext.J {
			// Self-loop: no intermediate steps to synthesize.
		} else if nodeCur.I == nodeNext.I {
			step := 1
			if nodeCur.J > nodeNext.J {
				step = -1
			}
			for j := nodeCur.J + step; j != nodeNext.J; j += step {
				if id, ok := byCoord[[2]int{nodeCur.I, j}]; ok {
					n := g.Nodes[id]
					if n.Type == NodeTypeInvisible {
						beatRow = append(beatRow, BeatInfo{NodeID: id, NodeType: NodeTypeInvisible, I: n.I, J: n.J})
					} else {
						beatRow = append(beatRow, BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: n.I, J: n.J})
					}
				} else {
					beatRow = append(beatRow, BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: nodeCur.I, J: j})
				}
			}
		} else if nodeCur.J == nodeNext.J {
			step := 1
			if nodeCur.I > nodeNext.I {
				step = -1
			}
			for i := nodeCur.I + step; i != nodeNext.I; i += step {
				if id, ok := byCoord[[2]int{i, nodeCur.J}]; ok {
					n := g.Nodes[id]
					if n.Type == NodeTypeInvisible {
						beatRow = append(beatRow, BeatInfo{NodeID: id, NodeType: NodeTypeInvisible, I: n.I, J: n.J})
					} else {
						beatRow = append(beatRow, BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: n.I, J: n.J})
					}
				} else {
					beatRow = append(beatRow, BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: i, J: nodeCur.J})
				}
			}
		}
		cur = next
	}
	return beatRow, isLoop, loopStart
}

// CalculateBeatRowFrom computes the beat row starting from the provided node ID
// without permanently modifying the graph's StartNodeID.
func (g *Graph) CalculateBeatRowFrom(start NodeID) ([]BeatInfo, bool, int) {
	prev := g.StartNodeID
	g.StartNodeID = start
	row, loop, idx := g.CalculateBeatRowUnbounded()
	g.StartNodeID = prev
	return row, loop, idx
}

func (g *Graph) getIntermediateGridPoints(node1I int, node1J int, node2I int, node2J int) []NodeID {
	var intermediateNodeIDs []NodeID
	g.logger.Tracef("[GRAPH] getIntermediateGridPoints: Calculating intermediate points between (%d,%d) and (%d,%d)", node1I, node1J, node2I, node2J)

	if node1I == node2I && node1J == node2J {
		return nil
	}

	if node1I == node2I { // Vertical line
		step := 1
		if node1J > node2J {
			step = -1
		}
		for j := node1J + step; j != node2J; j += step {
			foundIntermediateNodeID := InvalidNodeID
			for id, node := range g.Nodes {
				if node.I == node1I && node.J == j { // include any node type
					foundIntermediateNodeID = id
					break
				}
			}
			if foundIntermediateNodeID != InvalidNodeID {
				intermediateNodeIDs = append(intermediateNodeIDs, foundIntermediateNodeID)
				g.logger.Tracef("[GRAPH] getIntermediateGridPoints: Found intermediate node %d at (%d,%d)", foundIntermediateNodeID, node1I, j)
			} else {
				g.logger.Warnf("[GRAPH] Missing invisible node at (%d, %d) along vertical path", node1I, j)
			}
		}
	} else if node1J == node2J { // Horizontal line
		step := 1
		if node1I > node2I {
			step = -1
		}
		for i := node1I + step; i != node2I; i += step {
			foundIntermediateNodeID := InvalidNodeID
			for id, node := range g.Nodes {
				if node.I == i && node.J == node1J { // include any node type
					foundIntermediateNodeID = id
					break
				}
			}
			if foundIntermediateNodeID != InvalidNodeID {
				intermediateNodeIDs = append(intermediateNodeIDs, foundIntermediateNodeID)
				g.logger.Tracef("[GRAPH] getIntermediateGridPoints: Found intermediate node %d at (%d,%d)", foundIntermediateNodeID, i, node1J)
			} else {
				g.logger.Warnf("[GRAPH] Missing invisible node at (%d, %d) along horizontal path", i, node1J)
			}
		}
	}
	g.logger.Tracef("[GRAPH] getIntermediateGridPoints: Returning intermediateNodeIDs: %v", intermediateNodeIDs)
	return intermediateNodeIDs
}

func (g *Graph) IsLoop() bool {
	g.logger.Debugf("[GRAPH] IsLoop called. StartNodeID: %d, Nodes: %v, Edges: %v", g.StartNodeID, g.Nodes, g.Edges)

	visited := make(map[NodeID]bool)
	recStack := make(map[NodeID]bool)

	// Iterate over all nodes to handle disconnected components
	for nodeID := range g.Nodes {
		if !visited[nodeID] {
			g.logger.Tracef("[GRAPH] IsLoop: Starting DFS from node %d", nodeID)
			if g.dfsDetectCycle(nodeID, visited, recStack) {
				g.logger.Tracef("[GRAPH] IsLoop: Found a cycle, returning true")
				return true
			}
		}
	}

	g.logger.Debugf("[GRAPH] IsLoop: No cycle found, returning false")
	return false
}

func (g *Graph) dfsDetectCycle(nodeID NodeID, visited, recStack map[NodeID]bool) bool {
	g.logger.Tracef("[GRAPH] dfsDetectCycle: Visiting node %d. visited: %v, recStack: %v", nodeID, visited, recStack)
	visited[nodeID] = true
	recStack[nodeID] = true

	var neighbors []NodeID
	for edge := range g.Edges {
		if edge[0] == nodeID {
			neighbors = append(neighbors, edge[1])
		}
	}
	sort.Slice(neighbors, func(i, j int) bool {
		return neighbors[i] < neighbors[j]
	})

	for _, neighborID := range neighbors {
		g.logger.Tracef("[GRAPH] dfsDetectCycle: From node %d, checking neighbor %d", nodeID, neighborID)
		if !visited[neighborID] {
			if g.dfsDetectCycle(neighborID, visited, recStack) {
				return true
			}
		} else if recStack[neighborID] {
			g.logger.Tracef("[GRAPH] dfsDetectCycle: Found back edge to %d (cycle detected)", neighborID)
			return true // Found a cycle
		}
	}

	recStack[nodeID] = false
	g.logger.Tracef("[GRAPH] dfsDetectCycle: Backtracking from node %d. recStack: %v", nodeID, recStack)
	return false
}
