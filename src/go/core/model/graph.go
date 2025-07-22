package model

import (
	"sort"

	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

const InvalidNodeID NodeID = -1

type NodeID int

type Node struct{
	I, J int
	Type NodeType // New field: NodeType
}

// NodeType defines the type of a node.
type NodeType int

const (
	NodeTypeRegular NodeType = iota
	NodeTypeInvisible
)

// BeatInfo holds information about a beat in the drum row.
type BeatInfo struct {
	NodeID   NodeID
	NodeType NodeType
	I, J     int // Grid coordinates for this beat
}

type Graph struct {
	Nodes         map[NodeID]Node
	Edges         map[[2]NodeID]struct{}
	Next          NodeID
	Row           []bool
	StartNodeID   NodeID // ID of the explicit start node
	beatLengthValue int    // Desired length of the beat row
	logger        *game_log.Logger
}

func NewGraph(logger *game_log.Logger) *Graph {
	return &Graph{
		Nodes:           map[NodeID]Node{},
		Edges:           map[[2]NodeID]struct{}{},
		Next:            0,
		Row:             make([]bool, 4),
		StartNodeID:     InvalidNodeID, // Initialize with an invalid ID
		beatLengthValue: 16, // Default beat length
		logger:          logger,
	}
}

func (g *Graph) AddNode(i, j int, nodeType NodeType) NodeID {
	id := g.Next
	g.Next++
	g.Nodes[id] = Node{I: i, J: j, Type: nodeType}
	g.logger.Debugf("[GRAPH] Added node: %d at (%d, %d) with type %v", id, i, j, nodeType)
	return id
}

func (g *Graph) RemoveNode(id NodeID) {
	n := g.Nodes[id]
	delete(g.Nodes, id)
	for k := range g.Edges {
		if k[0] == id || k[1] == id {
			delete(g.Edges, k)
		}
	}
	g.logger.Debugf("[GRAPH] Removed node: %d at (%d, %d)", id, n.I, n.J)
}

func (g *Graph) ToggleStep(i int) {
	// This function will be re-evaluated later based on graph traversal
}

func (g *Graph) GetNodeByID(id NodeID) (Node, bool) {
	n, ok := g.Nodes[id]
	return n, ok
}

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

	path := []NodeID{}
	visited := make(map[NodeID]int)
	isLoop := false
	loopStartIndex := -1

	currentNodeID := g.StartNodeID
	g.logger.Debugf("[GRAPH] CalculateBeatRow: Starting traversal from node %d", currentNodeID)

	for currentNodeID != InvalidNodeID {
		g.logger.Debugf("[GRAPH] CalculateBeatRow: Current node: %d, Path so far: %v, Visited: %v", currentNodeID, path, visited)

		if index, ok := visited[currentNodeID]; ok {
			isLoop = true
			loopStartIndex = index
			// Do not append currentNodeID again, it's already in path at 'index'
			g.logger.Debugf("[GRAPH] CalculateBeatRow: Loop detected! Node %d revisited at index %d. Final path before break: %v", currentNodeID, index, path)
			break
		}

		visited[currentNodeID] = len(path)
		path = append(path, currentNodeID)

		var neighbors []NodeID
		for edge := range g.Edges {
			if edge[0] == currentNodeID {
				neighbors = append(neighbors, edge[1])
			}
		}

		if len(neighbors) == 0 {
			g.logger.Debugf("[GRAPH] CalculateBeatRow: No neighbors for node %d. Path ends.", currentNodeID)
			break
		}

		sort.Slice(neighbors, func(i, j int) bool {
			nodeA := g.Nodes[neighbors[i]]
			nodeB := g.Nodes[neighbors[j]]
			if nodeA.J != nodeB.J {
				return nodeA.J < nodeB.J
			}
			return nodeA.I < nodeB.I
		})
		nextNodeID := neighbors[0]
		g.logger.Debugf("[GRAPH] CalculateBeatRow: Next node selected: %d (from neighbors %v)", nextNodeID, neighbors)

		currentNode := g.Nodes[currentNodeID]
		nextNode := g.Nodes[nextNodeID]
		intermediateIDs := g.getIntermediateGridPoints(currentNode.I, currentNode.J, nextNode.I, nextNode.J)
		if len(intermediateIDs) > 0 {
			g.logger.Debugf("[GRAPH] CalculateBeatRow: Adding intermediate nodes: %v", intermediateIDs)
		}
		path = append(path, intermediateIDs...)
		currentNodeID = nextNodeID
	}

	beatRow := []BeatInfo{}
	for _, id := range path {
		if node, ok := g.Nodes[id]; ok {
			beatRow = append(beatRow, BeatInfo{NodeID: id, NodeType: node.Type, I: node.I, J: node.J})
		} else {
			g.logger.Warnf("[GRAPH] CalculateBeatRow: Node ID %d not found in graph.Nodes. Skipping.", id)
		}
	}
	g.logger.Debugf("[GRAPH] CalculateBeatRow: Raw beatRow before padding/loop handling: %v", beatRow)

	if isLoop {
		prefix := beatRow[:loopStartIndex]
		loopSegment := beatRow[loopStartIndex:] // Corrected: include the last element of the loop

		g.logger.Debugf("[GRAPH] CalculateBeatRow: Loop detected. Prefix: %v, Loop Segment: %v", prefix, loopSegment)

		finalBeatRow := []BeatInfo{}
		finalBeatRow = append(finalBeatRow, prefix...)

		if len(loopSegment) > 0 {
			for len(finalBeatRow) < g.beatLengthValue {
				finalBeatRow = append(finalBeatRow, loopSegment...)
			}
		} else {
			g.logger.Warnf("[GRAPH] CalculateBeatRow: Loop detected but loop segment is empty. This might indicate an issue in loop detection or graph structure.")
		}
		beatRow = finalBeatRow
		g.logger.Debugf("[GRAPH] CalculateBeatRow: BeatRow after loop expansion: %v", beatRow)
	}

	// Trim or pad the beatRow to the desired beatLengthValue
	if len(beatRow) > g.beatLengthValue {
		beatRow = beatRow[:g.beatLengthValue]
		g.logger.Debugf("[GRAPH] CalculateBeatRow: Trimmed beatRow to length %d: %v", g.beatLengthValue, beatRow)
	} else {
		// Pad if it's not a loop or if the loop expansion didn't fill it up
		for len(beatRow) < g.beatLengthValue {
			beatRow = append(beatRow, BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: -1, J: -1})
		}
		g.logger.Debugf("[GRAPH] CalculateBeatRow: Padded beatRow to length %d: %v", g.beatLengthValue, beatRow)
	}

	g.logger.Debugf("[GRAPH] CalculateBeatRow: End. Final beatRow length: %d, IsLoop: %t, BeatRow: %v", len(beatRow), isLoop, beatRow)
	return beatRow, isLoop, loopStartIndex
}

func (g *Graph) getIntermediateGridPoints(node1I int, node1J int, node2I int, node2J int) []NodeID {
	var intermediateNodeIDs []NodeID
	g.logger.Debugf("[GRAPH] getIntermediateGridPoints: Calculating intermediate points between (%d,%d) and (%d,%d)", node1I, node1J, node2I, node2J)

	if node1I == node2I { // Vertical line
		step := 1
		if node1J > node2J {
			step = -1
		}
		for j := node1J + step; j != node2J; j += step {
			foundIntermediateNodeID := InvalidNodeID
			for id, node := range g.Nodes {
				if node.I == node1I && node.J == j && node.Type == NodeTypeInvisible {
					foundIntermediateNodeID = id
					break
				}
			}
			if foundIntermediateNodeID != InvalidNodeID {
				intermediateNodeIDs = append(intermediateNodeIDs, foundIntermediateNodeID)
				g.logger.Debugf("[GRAPH] getIntermediateGridPoints: Found intermediate invisible node %d at (%d,%d)", foundIntermediateNodeID, node1I, j)
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
				if node.I == i && node.J == node1J && node.Type == NodeTypeInvisible {
					foundIntermediateNodeID = id
					break
				}
			}
			if foundIntermediateNodeID != InvalidNodeID {
				intermediateNodeIDs = append(intermediateNodeIDs, foundIntermediateNodeID)
				g.logger.Debugf("[GRAPH] getIntermediateGridPoints: Found intermediate invisible node %d at (%d,%d)", foundIntermediateNodeID, i, node1J)
			} else {
				g.logger.Warnf("[GRAPH] Missing invisible node at (%d, %d) along horizontal path", i, node1J)
			}
		}
	}
	g.logger.Debugf("[GRAPH] getIntermediateGridPoints: Returning intermediateNodeIDs: %v", intermediateNodeIDs)
	return intermediateNodeIDs
}


func (g *Graph) IsLoop() bool {
	g.logger.Debugf("[GRAPH] IsLoop called. StartNodeID: %d, Nodes: %v, Edges: %v", g.StartNodeID, g.Nodes, g.Edges)

	visited := make(map[NodeID]bool)
	recStack := make(map[NodeID]bool)

	// Iterate over all nodes to handle disconnected components
	for nodeID := range g.Nodes {
		if !visited[nodeID] {
			g.logger.Debugf("[GRAPH] IsLoop: Starting DFS from node %d", nodeID)
			if g.dfsDetectCycle(nodeID, visited, recStack) {
				g.logger.Debugf("[GRAPH] IsLoop: Found a cycle, returning true")
				return true
			}
		}
	}

	g.logger.Debugf("[GRAPH] IsLoop: No cycle found, returning false")
	return false
}

func (g *Graph) dfsDetectCycle(nodeID NodeID, visited, recStack map[NodeID]bool) bool {
	g.logger.Debugf("[GRAPH] dfsDetectCycle: Visiting node %d. visited: %v, recStack: %v", nodeID, visited, recStack)
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
		g.logger.Debugf("[GRAPH] dfsDetectCycle: From node %d, checking neighbor %d", nodeID, neighborID)
		if !visited[neighborID] {
			if g.dfsDetectCycle(neighborID, visited, recStack) {
				return true
			}
		} else if recStack[neighborID] {
			g.logger.Debugf("[GRAPH] dfsDetectCycle: Found back edge to %d (cycle detected)", neighborID)
			return true // Found a cycle
		}
	}

	recStack[nodeID] = false
	g.logger.Debugf("[GRAPH] dfsDetectCycle: Backtracking from node %d. recStack: %v", nodeID, recStack)
	return false
}

func (g *Graph) SetBeatLength(length int) {
	g.beatLengthValue = length
}

func (g *Graph) BeatLength() int {
	return g.beatLengthValue
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}
