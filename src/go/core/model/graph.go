package model

type NodeID int

type Node struct{ I, J int }

type Graph struct {
	Nodes map[NodeID]Node
	Edges map[[2]NodeID]struct{}
	Next  NodeID
	Row   []bool
}

func NewGraph() *Graph {
	return &Graph{
		Nodes: map[NodeID]Node{},
		Edges: map[[2]NodeID]struct{}{},
		Row:   make([]bool, 4),
	}
}

func (g *Graph) AddNode(i, j int) NodeID {
	id := g.Next
	g.Next++
	g.Nodes[id] = Node{I: i, J: j}
	g.ensureLen(i + 1)
	if j == 0 {
		g.Row[i] = true
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
	if n.J == 0 {
		g.Row[n.I] = false
	}
}

func (g *Graph) ToggleStep(i int) {
	g.ensureLen(i + 1)
	g.Row[i] = !g.Row[i]
	if g.Row[i] {
		g.AddNode(i, 0)
	} else {
		for id, n := range g.Nodes {
			if n.I == i && n.J == 0 {
				g.RemoveNode(id)
			}
		}
	}
}

func (g *Graph) ensureLen(n int) {
	if n <= len(g.Row) {
		return
	}
	tmp := make([]bool, n)
	copy(tmp, g.Row)
	g.Row = tmp
}
