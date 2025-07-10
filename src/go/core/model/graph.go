package model

import (
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

const InvalidNodeID NodeID = -1

type NodeID int

type Node struct{ I, J int }

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

func (g *Graph) AddNode(i, j int) NodeID {
	id := g.Next
	g.Next++
	g.Nodes[id] = Node{I: i, J: j}
	g.logger.Debugf("[GRAPH] Added node: %d at (%d, %d)", id, i, j)
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

func (g *Graph) CalculateBeatRow() ([]bool, map[int]NodeID) {
	beatRow := make([]bool, g.beatLengthValue)
	activeNodes := make(map[int]NodeID) // New map to store active nodes at each step

	if g.StartNodeID == InvalidNodeID {
		return beatRow, activeNodes
	}

	_, ok := g.Nodes[g.StartNodeID]
	if !ok {
		return beatRow, activeNodes
	}

	// Use a queue for BFS, storing (NodeID, Node, distance)
	queue := []struct {
		id       NodeID
		node     Node
		distance int
	}{{g.StartNodeID, g.Nodes[g.StartNodeID], 0}}
	visited := make(map[NodeID]bool)
	visited[g.StartNodeID] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Mark the beat at the calculated distance
		g.logger.Debugf("[GRAPH] BFS: currentID=%d, node=(%d,%d), distance=%d", current.id, current.node.I, current.node.J, current.distance)
		if current.distance < g.beatLengthValue {
			beatRow[current.distance] = true
			activeNodes[current.distance] = current.id // Store the node ID for this step
		}

		// Find neighbors
		for edge := range g.Edges {
			if edge[0] == current.id {
				neighborID := edge[1]
				neighborNode := g.Nodes[neighborID]
				g.logger.Debugf("[GRAPH] BFS: Found neighbor: %d (%d,%d) for %d (%d,%d)", neighborID, neighborNode.I, neighborNode.J, current.id, current.node.I, current.node.J)
				if !visited[neighborID] {
					visited[neighborID] = true
					// Calculate beats based on Manhattan distance
					beats := abs(current.node.I-neighborNode.I) + abs(current.node.J-neighborNode.J)
					g.logger.Debugf("[GRAPH] BFS: Calculated beats: between (%d,%d) and (%d,%d) = %d", current.node.I, current.node.J, neighborNode.I, neighborNode.J, beats)
					queue = append(queue, struct {
						id       NodeID
						node     Node
						distance int
					}{neighborID, neighborNode, current.distance + beats})
				}
			}
		}
	}

	g.logger.Debugf("[GRAPH] Calculated beat row: %v (startNodeID: %d)", beatRow, g.StartNodeID)
	return beatRow, activeNodes
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
