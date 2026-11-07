package ui

type healClass int

const (
	// healSilent: ≤1 edge — extraction disconnects nothing else; no heal.
	healSilent healClass = iota
	// healPassThrough: exactly Np→N→Nn with Np,Nn orthogonal — heal Np→Nn.
	healPassThrough
	// Extraction breaks the circuit; interactive callers must confirm.
	healUnhealable
)

// healPlan is the pre-computed extraction plan for moving a node.
type healPlan struct {
	class healClass
	from  *uiNode // Np (healPassThrough only)
	to    *uiNode // Nn (healPassThrough only)
}

// Pure. Incoming: e.B == n; outgoing: e.A == n.
func (g *Game) moveHealPlan(n *uiNode) healPlan {
	var ins, outs []*uiNode
	for i := range g.edges {
		e := &g.edges[i]
		if e.A.ID == n.ID && e.B.ID == n.ID {
			// Import can create a self-loop: it moves with the node and never heals.
			continue
		}
		switch {
		case e.A.ID == n.ID:
			outs = append(outs, e.B)
		case e.B.ID == n.ID:
			ins = append(ins, e.A)
		}
	}
	if len(ins)+len(outs) <= 1 {
		return healPlan{class: healSilent}
	}
	if len(ins) == 1 && len(outs) == 1 && ins[0].ID != outs[0].ID {
		np, nn := ins[0], outs[0]
		if np.I == nn.I || np.J == nn.J {
			return healPlan{class: healPassThrough, from: np, to: nn}
		}
	}
	return healPlan{class: healUnhealable}
}

// Callers stitch each, so a healed edge stays split node to node.
func (g *Game) nodesOnSpan(a, b *uiNode) []*uiNode {
	var out []*uiNode
	switch {
	case a.I == b.I:
		lo, hi := min(a.J, b.J), max(a.J, b.J)
		for _, m := range g.nodes {
			if m.I == a.I && m.J > lo && m.J < hi {
				out = append(out, m)
			}
		}
	case a.J == b.J:
		lo, hi := min(a.I, b.I), max(a.I, b.I)
		for _, m := range g.nodes {
			if m.J == a.J && m.I > lo && m.I < hi {
				out = append(out, m)
			}
		}
	}
	return out
}
