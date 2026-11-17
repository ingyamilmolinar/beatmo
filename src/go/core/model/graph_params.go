package model

// SetNodeParams updates the playback parameters for a node. Zero values for
// Volume and Duration are interpreted as identity (1). Pitch is absolute.
func (g *Graph) SetNodeParams(id NodeID, p NodeParams) {
	n, ok := g.Nodes[id]
	if !ok {
		return
	}
	// Do not coerce zero values; allow explicit 0 volume/duration.
	n.Params.Volume = p.Volume
	n.Params.Pitch = p.Pitch
	n.Params.Duration = p.Duration
	n.Params.Logic = p.Logic
	n.Params.LogicKind = p.LogicKind
	n.Params.LogicN = p.LogicN
	n.Params.LogicP = p.LogicP
	n.Params.GrooveKind = p.GrooveKind
	n.Params.GroovePct = p.GroovePct
	n.Params.EffectOverrides = p.EffectOverrides
	n.Params.Chord = p.Chord
	g.Nodes[id] = n
	if g.onNodeChanged != nil {
		g.onNodeChanged(id)
	}
}

// SetNodeLogic attaches a logic callback to a node.
func (g *Graph) SetNodeLogic(id NodeID, logic NodeLogic) {
	n, ok := g.Nodes[id]
	if !ok {
		return
	}
	n.Params.Logic = logic
	g.Nodes[id] = n
	if g.onNodeChanged != nil {
		g.onNodeChanged(id)
	}
}
