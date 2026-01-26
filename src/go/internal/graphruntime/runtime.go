package graphruntime

import (
	"fmt"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// NodeSnapshot captures an immutable view of a graph node.
type NodeSnapshot struct {
	ID   model.NodeID
	Node model.Node
}

// EdgeSnapshot captures a directed edge between two nodes.
type EdgeSnapshot struct {
	From model.NodeID
	To   model.NodeID
}

// GraphSnapshot is an immutable view of the underlying graph.
type GraphSnapshot struct {
	Nodes []NodeSnapshot
	Edges []EdgeSnapshot
}

// Command applies a mutation against the runtime's backing graph.
type Command interface {
	Apply(rt *Runtime) error
}

// Runtime wraps a core graph and exposes immutable snapshots plus edit commands.
type Runtime struct {
	graph *model.Graph
}

// NewRuntime creates a graph runtime wrapper.
func NewRuntime(graph *model.Graph) *Runtime {
	if graph == nil {
		panic("graphruntime: graph must not be nil")
	}
	return &Runtime{graph: graph}
}

// Graph returns the underlying mutable graph. Use cautiously.
func (rt *Runtime) Graph() *model.Graph {
	return rt.graph
}

// Snapshot returns an immutable view of the graph at this instant.
func (rt *Runtime) Snapshot() GraphSnapshot {
	nodes := make([]NodeSnapshot, 0, len(rt.graph.Nodes))
	for id, node := range rt.graph.Nodes {
		nodes = append(nodes, NodeSnapshot{ID: id, Node: node})
	}
	edges := make([]EdgeSnapshot, 0, len(rt.graph.Edges))
	for edge := range rt.graph.Edges {
		edges = append(edges, EdgeSnapshot{From: edge[0], To: edge[1]})
	}
	return GraphSnapshot{
		Nodes: nodes,
		Edges: edges,
	}
}

// Apply executes a single command.
func (rt *Runtime) Apply(cmd Command) error {
	if cmd == nil {
		return fmt.Errorf("graphruntime: command is nil")
	}
	return cmd.Apply(rt)
}

// ApplyAll executes commands sequentially, stopping on the first error.
func (rt *Runtime) ApplyAll(cmds ...Command) error {
	for _, cmd := range cmds {
		if err := rt.Apply(cmd); err != nil {
			return err
		}
	}
	return nil
}

// --- Commands ----------------------------------------------------------------

// AddNodeCommand inserts a node at grid coordinates with the given type.
type AddNodeCommand struct {
	I, J int
	Type model.NodeType

	outID *model.NodeID
}

// Apply executes the add node command.
func (cmd *AddNodeCommand) Apply(rt *Runtime) error {
	id := rt.graph.AddNode(cmd.I, cmd.J, cmd.Type)
	if cmd.outID != nil {
		*cmd.outID = id
	}
	return nil
}

// CaptureID writes the created node ID into target after Apply.
func (cmd *AddNodeCommand) CaptureID(target *model.NodeID) *AddNodeCommand {
	cmd.outID = target
	return cmd
}

// RemoveNodeCommand deletes a node and associated edges.
type RemoveNodeCommand struct {
	ID model.NodeID
}

func (cmd *RemoveNodeCommand) Apply(rt *Runtime) error {
	rt.graph.RemoveNode(cmd.ID)
	return nil
}

// SetNodeParamsCommand updates parameters for a node.
type SetNodeParamsCommand struct {
	ID model.NodeID
	P  model.NodeParams
}

func (cmd *SetNodeParamsCommand) Apply(rt *Runtime) error {
	rt.graph.SetNodeParams(cmd.ID, cmd.P)
	return nil
}

// AddEdgeCommand adds a directed edge.
type AddEdgeCommand struct {
	From model.NodeID
	To   model.NodeID
}

func (cmd *AddEdgeCommand) Apply(rt *Runtime) error {
	rt.graph.Edges[[2]model.NodeID{cmd.From, cmd.To}] = struct{}{}
	return nil
}

// RemoveEdgeCommand removes a directed edge if present.
type RemoveEdgeCommand struct {
	From model.NodeID
	To   model.NodeID
}

func (cmd *RemoveEdgeCommand) Apply(rt *Runtime) error {
	delete(rt.graph.Edges, [2]model.NodeID{cmd.From, cmd.To})
	return nil
}
