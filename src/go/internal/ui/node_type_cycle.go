package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Regular → Chord → Silent → Mute; Chord is skipped on an unpitched instrument.
func nextNodeType(cur model.NodeType, pitched bool) model.NodeType {
	switch cur {
	case model.NodeTypeRegular:
		if pitched {
			return model.NodeTypeChord
		}
		return model.NodeTypeSilent
	case model.NodeTypeChord:
		return model.NodeTypeSilent
	case model.NodeTypeSilent:
		return model.NodeTypeMute
	default:
		return model.NodeTypeRegular
	}
}

// The same test the Pitch chip uses.
func (g *Game) nodeIsPitched(id model.NodeID) bool {
	n, ok := g.graph.GetNodeByID(id)
	if !ok {
		return false
	}
	return g.nodeNoteLabel(id, n.Params.Pitch) != ""
}

// cycleNodeType advances the node along the type cycle.
func (g *Game) cycleNodeType(id model.NodeID) {
	n, ok := g.graph.GetNodeByID(id)
	if !ok {
		return
	}
	g.setNodeType(id, nextNodeType(n.Type, g.nodeIsPitched(id)))
}

// Entering Chord with no spec seeds a triad in the same SetNodeParams (one undo step); otherwise the
// spec stays, so a collapsed chord comes back as it was.
func (g *Game) setNodeType(id model.NodeID, t model.NodeType) {
	n, ok := g.graph.GetNodeByID(id)
	if !ok || n.Type == t {
		return
	}
	old := n.Type
	n.Type = t
	g.graph.Nodes[id] = n
	if t == model.NodeTypeChord && n.Params.Chord.IsZero() {
		p := n.Params
		p.Chord = model.ChordSpec{Quality: model.ChordDefaultQuality}
		g.graph.SetNodeParams(id, p)
	}
	if t == model.NodeTypeChord && g.sidebar != nil {
		g.sidebar.sectionOpen[secChord] = true
	}
	g.cacheNode(id)
	g.notifyPredictorNode(id)
	g.updateBeatInfos()
	emitNodeTypeChanged(id, old, t)
}

// The chord builder's single mutation path; one undo step.
func (g *Game) setNodeChord(id model.NodeID, c model.ChordSpec) {
	n, ok := g.graph.GetNodeByID(id)
	if !ok {
		return
	}
	p := n.Params
	p.Chord = model.NormalizeChord(c)
	g.graph.SetNodeParams(id, p)
	g.notifyPredictorNode(id)
	emitNodeChordChanged(id, p, g.nodeChordLabel(id))
}

// Clamped to the quality.
func (g *Game) stepNodeChord(id model.NodeID, field sbField, delta int) {
	n, ok := g.graph.GetNodeByID(id)
	if !ok {
		return
	}
	c := n.Params.Chord
	if c.Quality == "" {
		c.Quality = model.ChordDefaultQuality
	}
	switch field {
	case fieldChordInv:
		c.Inversion += delta
	case fieldChordSpread:
		c.Spread += delta
	}
	if model.NormalizeChord(c) == model.NormalizeChord(n.Params.Chord) {
		return // at a limit: no change, no undo step
	}
	g.setNodeChord(id, c)
}

// Rooted on the note the Pitch chip names.
func (g *Game) nodeChordLabel(id model.NodeID) string {
	n, ok := g.graph.GetNodeByID(id)
	if !ok {
		return ""
	}
	return model.ChordName(g.nodeRootMIDI(id, n.Params.Pitch), n.Params.Chord)
}

// Offset 0 when unpitched.
func (g *Game) nodeRootMIDI(id model.NodeID, pitch float64) int {
	offset := 0.0
	if row, ok := g.nodeRows[id]; ok && row >= 0 && row < len(g.drum.Rows) {
		if o, pitched := audio.InstrumentPitchInfo(g.drum.Rows[row].Instrument); pitched {
			offset = o
		}
	}
	return nodePitchMIDI(pitch, offset)
}

func chordInversionLabel(k int) string {
	switch k {
	case 1:
		return i18n.T(i18n.KeyChordInv1)
	case 2:
		return i18n.T(i18n.KeyChordInv2)
	case 3:
		return i18n.T(i18n.KeyChordInv3)
	case 4:
		return i18n.T(i18n.KeyChordInv4)
	}
	return i18n.T(i18n.KeyChordInvRoot)
}

func chordSpreadLabel(s int) string {
	switch s {
	case model.ChordSpreadDrop2:
		return i18n.T(i18n.KeyChordSpreadDrop2)
	case model.ChordSpreadWide:
		return i18n.T(i18n.KeyChordSpreadWide)
	}
	return i18n.T(i18n.KeyChordSpreadClose)
}
