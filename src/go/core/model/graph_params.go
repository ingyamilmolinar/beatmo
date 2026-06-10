package model

import "strings"

func normalizeNodeParams(p NodeParams) NodeParams {
	kind := strings.ToLower(strings.TrimSpace(p.LogicKind))
	switch kind {
	case "none":
		kind = ""
	case "prev_fired":
		kind = "trigger_if_prev_triggered"
	case "every_n_loops":
		kind = "every_n_triggers"
	case "skip_if_prev_skipped":
		kind = "trigger_if_prev_triggered"
	case "skip_if_prev_triggered":
		kind = "trigger_if_prev_skipped"
	}
	p.LogicKind = kind
	return p
}

// SetNodeParams updates the playback parameters for a node. Zero values for
// Volume and Duration are interpreted as identity (1). Pitch is absolute.
func (g *Graph) SetNodeParams(id NodeID, p NodeParams) {
	n, ok := g.Nodes[id]
	if !ok {
		return
	}
	p = normalizeNodeParams(p)
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
	n.Params.SynthDecay = p.SynthDecay
	n.Params.SynthTone = p.SynthTone
	n.Params.SynthAttack = p.SynthAttack
	n.Params.SynthDrive = p.SynthDrive
	n.Params.SynthBody = p.SynthBody
	n.Params.SynthColor = p.SynthColor
	n.Params.SynthBrightness = p.SynthBrightness
	n.Params.EffectOverrides = p.EffectOverrides
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
