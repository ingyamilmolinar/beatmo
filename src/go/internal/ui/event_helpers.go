package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// emitNodeAdded publishes EventNodeAdded with the node's grid coordinates
// and type. Called from tryAddNode after the graph mutation succeeds.
func emitNodeAdded(n *uiNode, t model.NodeType) {
	if n == nil {
		return
	}
	hooks.PublishKind(hooks.EventNodeAdded, hooks.NodeEdit{
		ID:   int(n.ID),
		I:    n.I,
		J:    n.J,
		Type: nodeTypeName(t),
	})
}

// emitNodeDeleted publishes EventNodeDeleted with the deleted node's
// last-known coordinates. Called from deleteNodeInternal after the
// graph mutation completes.
func emitNodeDeleted(id model.NodeID, i, j int) {
	hooks.PublishKind(hooks.EventNodeDeleted, hooks.NodeEdit{
		ID: int(id), I: i, J: j,
	})
}

// emitNodeMoved publishes EventNodeMoved on drag-end.
func emitNodeMoved(id model.NodeID, i, j int) {
	hooks.PublishKind(hooks.EventNodeMoved, hooks.NodeEdit{
		ID: int(id), I: i, J: j,
	})
}

// emitNodeTypeChanged publishes EventNodeTypeChanged when a node's type
// is mutated (Regular ↔ Silent ↔ Mute ↔ Invisible).
func emitNodeTypeChanged(id model.NodeID, oldT, newT model.NodeType) {
	hooks.PublishKind(hooks.EventNodeTypeChanged, hooks.NodeTypePayload{
		ID:      int(id),
		OldType: nodeTypeName(oldT),
		NewType: nodeTypeName(newT),
	})
}

// emitNodeParamsChanged publishes EventNodeParamsChanged with the full
// new parameter snapshot. Subscribers can diff against their own copy
// to identify which field changed.
func emitNodeParamsChanged(id model.NodeID, p model.NodeParams) {
	hooks.PublishKind(hooks.EventNodeParamsChanged, hooks.NodeParamsPayload{
		ID:         int(id),
		Volume:     p.Volume,
		Pitch:      p.Pitch,
		Duration:   p.Duration,
		LogicKind:  p.LogicKind,
		LogicN:     p.LogicN,
		LogicP:     p.LogicP,
		GrooveKind: p.GrooveKind,
		GroovePct:  p.GroovePct,
	})
}

// emitStartNodeChanged publishes EventStartNodeChanged when a row's
// origin node is reassigned.
func emitStartNodeChanged(row int, id model.NodeID) {
	hooks.PublishKind(hooks.EventStartNodeChanged, hooks.StartNodePayload{
		Row: row, ID: int(id),
	})
}

// emitEdgeAdded publishes EventEdgeAdded with both endpoints.
func emitEdgeAdded(fromID, toID model.NodeID, fromI, fromJ, toI, toJ int) {
	hooks.PublishKind(hooks.EventEdgeAdded, hooks.EdgeEdit{
		FromID: int(fromID), ToID: int(toID),
		FromI: fromI, FromJ: fromJ, ToI: toI, ToJ: toJ,
	})
}

// emitEdgeDeleted publishes EventEdgeDeleted with both endpoints.
func emitEdgeDeleted(fromID, toID model.NodeID, fromI, fromJ, toI, toJ int) {
	hooks.PublishKind(hooks.EventEdgeDeleted, hooks.EdgeEdit{
		FromID: int(fromID), ToID: int(toID),
		FromI: fromI, FromJ: fromJ, ToI: toI, ToJ: toJ,
	})
}

// emitSeek publishes EventSeek with the destination beat index.
func emitSeek(beats int) {
	hooks.PublishKind(hooks.EventSeek, hooks.SeekPayload{Beats: beats})
}

// emitSubdivChange publishes EventSubdivChange.
func emitSubdivChange(subdiv int) {
	hooks.PublishKind(hooks.EventSubdivChange, hooks.SubdivPayload{Subdiv: subdiv})
}

// emitLengthChange publishes EventLengthChange.
func emitLengthChange(length int) {
	hooks.PublishKind(hooks.EventLengthChange, hooks.LengthPayload{Length: length})
}

// emitRowAdded publishes EventRowAdded.
func emitRowAdded(row int, instrument, name string) {
	hooks.PublishKind(hooks.EventRowAdded, hooks.RowChangePayload{
		Row: row, Instrument: instrument, Name: name,
	})
}

// emitRowDeleted publishes EventRowDeleted.
func emitRowDeleted(row int) {
	hooks.PublishKind(hooks.EventRowDeleted, hooks.RowChangePayload{Row: row})
}

// emitRowInstrumentChange publishes EventRowInstrumentChange. oldInstrument
// is the instrument id the row was set to before the change.
func emitRowInstrumentChange(row int, oldInstrument, instrument, name string) {
	hooks.PublishKind(hooks.EventRowInstrumentChange, hooks.RowChangePayload{
		Row:           row,
		OldInstrument: oldInstrument,
		Instrument:    instrument,
		Name:          name,
	})
}

// emitRowMute publishes EventRowMute.
func emitRowMute(row int, mute bool) {
	hooks.PublishKind(hooks.EventRowMute, hooks.RowChangePayload{Row: row, Mute: mute})
}

// emitRowSolo publishes EventRowSolo.
func emitRowSolo(row int, solo bool) {
	hooks.PublishKind(hooks.EventRowSolo, hooks.RowChangePayload{Row: row, Solo: solo})
}

// emitMasterVolumeChange publishes EventMasterVolumeChange. Called from
// the master volume slider's commit handler.
func emitMasterVolumeChange(v float64) {
	hooks.PublishKind(hooks.EventMasterVolumeChange, hooks.MasterVolumePayload{Volume: v})
}

// nodeTypeName converts a model.NodeType to its lowercase string label
// matching the JSON schema in CLAUDE.md.
func nodeTypeName(t model.NodeType) string {
	switch t {
	case model.NodeTypeRegular:
		return "regular"
	case model.NodeTypeInvisible:
		return "invisible"
	case model.NodeTypeSilent:
		return "silent"
	case model.NodeTypeMute:
		return "mute"
	}
	return "unknown"
}
