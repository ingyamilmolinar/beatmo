package model

import game_log "github.com/ingyamilmolinar/tunkul/internal/log"

type Graph struct {
	Nodes           map[NodeID]Node
	Edges           map[[2]NodeID]struct{}
	Next            NodeID
	Row             []bool
	StartNodeID     NodeID // ID of the explicit start node
	beatLengthValue int    // Desired length of the beat row
	logger          *game_log.Logger
	onNodeChanged   func(NodeID)
}

func NewGraph(logger *game_log.Logger) *Graph {
	return &Graph{
		Nodes:           map[NodeID]Node{},
		Edges:           map[[2]NodeID]struct{}{},
		Next:            0,
		Row:             make([]bool, 4),
		StartNodeID:     InvalidNodeID, // Initialize with an invalid ID
		beatLengthValue: 16,            // Default beat length
		logger:          logger,
	}
}

// SetNodeChangedHook registers a callback invoked whenever a node's parameters
// or type are modified through Graph helpers.
func (g *Graph) SetNodeChangedHook(fn func(NodeID)) {
	g.onNodeChanged = fn
}

func (g *Graph) AddNode(i, j int, nodeType NodeType) NodeID {
	id := g.Next
	g.Next++
	// Initialize sensible defaults for params.
	g.Nodes[id] = Node{I: i, J: j, Type: nodeType, Params: NodeParams{Volume: 1, Duration: 1}}
	g.logger.Debugf("[GRAPH] Added node: %d at (%d, %d) with type %v", id, i, j, nodeType)
	if g.onNodeChanged != nil {
		g.onNodeChanged(id)
	}
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
	if g.onNodeChanged != nil {
		g.onNodeChanged(id)
	}
}

func (g *Graph) ToggleStep(i int) {
	// This function will be re-evaluated later based on graph traversal
}

func (g *Graph) GetNodeByID(id NodeID) (Node, bool) {
	n, ok := g.Nodes[id]
	return n, ok
}

func (g *Graph) SetBeatLength(length int) {
	g.beatLengthValue = length
}

func (g *Graph) BeatLength() int {
	return g.beatLengthValue
}
