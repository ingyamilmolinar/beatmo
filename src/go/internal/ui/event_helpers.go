package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// All helpers in this file capture their caller's source via
// hooks.CaptureSource(1) and publish through hooks.PublishWithSource so
// subscribers (eventlogger, eventstream) receive the originating user-code
// site rather than this file. skip=1 means "my caller" — the user-action
// handler that invoked the helper.
//
// If a helper is itself wrapped by another helper, the outer helper must
// either inline the captureSource(1) call OR bump skip explicitly. Today
// no helper wraps another, so skip=1 is universally correct.

// emitNodeAdded publishes EventNodeAdded with the node's grid coordinates
// and type. Called from tryAddNode after the graph mutation succeeds.
func emitNodeAdded(n *uiNode, t model.NodeType) {
	if n == nil {
		return
	}
	hooks.PublishWithSource(hooks.EventNodeAdded, hooks.NodeEdit{
		ID:   int(n.ID),
		I:    n.I,
		J:    n.J,
		Type: nodeTypeName(t),
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodeAdded)
}

// emitNodeDeleted publishes EventNodeDeleted with the deleted node's
// last-known coordinates. Called from deleteNodeInternal after the
// graph mutation completes.
func emitNodeDeleted(id model.NodeID, i, j int) {
	hooks.PublishWithSource(hooks.EventNodeDeleted, hooks.NodeEdit{
		ID: int(id), I: i, J: j,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodeDeleted)
}

// emitNodeMoved publishes EventNodeMoved on drag-end.
func emitNodeMoved(id model.NodeID, i, j int) {
	hooks.PublishWithSource(hooks.EventNodeMoved, hooks.NodeEdit{
		ID: int(id), I: i, J: j,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodeMoved)
}

// emitNodeTypeChanged publishes EventNodeTypeChanged when a node's type
// is mutated (Regular ↔ Silent ↔ Mute ↔ Invisible).
func emitNodeTypeChanged(id model.NodeID, oldT, newT model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodeTypeChanged, hooks.NodeTypePayload{
		ID:      int(id),
		OldType: nodeTypeName(oldT),
		NewType: nodeTypeName(newT),
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodeTypeChanged)
}

// emitNodeParamsChanged publishes EventNodeParamsChanged with the full
// new parameter snapshot. Subscribers can diff against their own copy
// to identify which field changed.
func emitNodeParamsChanged(id model.NodeID, p model.NodeParams) {
	hooks.PublishWithSource(hooks.EventNodeParamsChanged, hooks.NodeParamsPayload{
		ID:         int(id),
		Volume:     p.Volume,
		Pitch:      p.Pitch,
		Duration:   p.Duration,
		LogicKind:  p.LogicKind,
		LogicN:     p.LogicN,
		LogicP:     p.LogicP,
		GrooveKind: p.GrooveKind,
		GroovePct:  p.GroovePct,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodeParamsChanged)
}

// emitStartNodeChanged publishes EventStartNodeChanged when a row's
// origin node is reassigned.
func emitStartNodeChanged(row int, id model.NodeID) {
	hooks.PublishWithSource(hooks.EventStartNodeChanged, hooks.StartNodePayload{
		Row: row, ID: int(id),
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventStartNodeChanged)
}

// emitEdgeAdded publishes EventEdgeAdded with both endpoints.
func emitEdgeAdded(fromID, toID model.NodeID, fromI, fromJ, toI, toJ int) {
	hooks.PublishWithSource(hooks.EventEdgeAdded, hooks.EdgeEdit{
		FromID: int(fromID), ToID: int(toID),
		FromI: fromI, FromJ: fromJ, ToI: toI, ToJ: toJ,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventEdgeAdded)
}

// emitEdgeDeleted publishes EventEdgeDeleted with both endpoints.
func emitEdgeDeleted(fromID, toID model.NodeID, fromI, fromJ, toI, toJ int) {
	hooks.PublishWithSource(hooks.EventEdgeDeleted, hooks.EdgeEdit{
		FromID: int(fromID), ToID: int(toID),
		FromI: fromI, FromJ: fromJ, ToI: toI, ToJ: toJ,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventEdgeDeleted)
}

// emitSeek publishes EventSeek with the destination beat index.
func emitSeek(beats int) {
	hooks.PublishWithSource(hooks.EventSeek, hooks.SeekPayload{Beats: beats}, hooks.CaptureSource(1))
}

// emitSubdivChange publishes EventSubdivChange.
func emitSubdivChange(subdiv int) {
	hooks.PublishWithSource(hooks.EventSubdivChange, hooks.SubdivPayload{Subdiv: subdiv}, hooks.CaptureSource(1))
	recordUndo(hooks.EventSubdivChange)
}

// emitLengthChange publishes EventLengthChange. Length is derived display state
// (re-computed from the graph beat-path; not in the export schema), so it is
// intentionally NOT tapped for undo — see TestUndoRecordedSetExcludesNonDocument.
func emitLengthChange(length int) {
	hooks.PublishWithSource(hooks.EventLengthChange, hooks.LengthPayload{Length: length}, hooks.CaptureSource(1))
}

// emitRowAdded publishes EventRowAdded.
func emitRowAdded(row int, instrument, name string) {
	hooks.PublishWithSource(hooks.EventRowAdded, hooks.RowChangePayload{
		Row: row, Instrument: instrument, Name: name,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventRowAdded)
}

// emitRowDeleted publishes EventRowDeleted.
func emitRowDeleted(row int) {
	hooks.PublishWithSource(hooks.EventRowDeleted, hooks.RowChangePayload{Row: row}, hooks.CaptureSource(1))
	recordUndo(hooks.EventRowDeleted)
}

// emitRowInstrumentChange publishes EventRowInstrumentChange. oldInstrument
// is the instrument id the row was set to before the change.
func emitRowInstrumentChange(row int, oldInstrument, instrument, name string) {
	hooks.PublishWithSource(hooks.EventRowInstrumentChange, hooks.RowChangePayload{
		Row:           row,
		OldInstrument: oldInstrument,
		Instrument:    instrument,
		Name:          name,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventRowInstrumentChange)
}

// emitRowMute publishes EventRowMute. Mute is session state (not in the export
// schema), so it is intentionally NOT tapped for undo — see documentScopeKinds
// and TestMuteSoloNotUndoableByDesign.
func emitRowMute(row int, mute bool) {
	hooks.PublishWithSource(hooks.EventRowMute, hooks.RowChangePayload{Row: row, Mute: mute}, hooks.CaptureSource(1))
}

// emitRowSolo publishes EventRowSolo. Solo is session state (not in the export
// schema), so it is intentionally NOT tapped for undo.
func emitRowSolo(row int, solo bool) {
	hooks.PublishWithSource(hooks.EventRowSolo, hooks.RowChangePayload{Row: row, Solo: solo}, hooks.CaptureSource(1))
}

// emitMasterVolumeChange publishes EventMasterVolumeChange. Called from
// the master volume slider's commit handler.
func emitMasterVolumeChange(v float64) {
	hooks.PublishWithSource(hooks.EventMasterVolumeChange, hooks.MasterVolumePayload{Volume: v}, hooks.CaptureSource(1))
}

// emitBPMChange publishes EventBPMChange with the typed payload. Replaces
// inline hooks.PublishKind(EventBPMChange, bpm) calls that used a raw int.
func emitBPMChange(bpm int) {
	hooks.PublishWithSource(hooks.EventBPMChange, hooks.BPMPayload{BPM: bpm}, hooks.CaptureSource(1))
	recordUndo(hooks.EventBPMChange)
}

// emitImport publishes EventImport with the typed payload (byte count + node
// count + row count). Replaces the legacy raw-int (bytes-only) payload.
func emitImport(bytes, nodes, rows int) {
	hooks.PublishWithSource(hooks.EventImport, hooks.ImportPayload{
		Bytes: bytes, Nodes: nodes, Rows: rows,
	}, hooks.CaptureSource(1))
}

// emitRowColorChanged publishes EventRowColorChanged. Coalesced (color-wheel
// drag emits per-frame; the trailing pick is the meaningful event).
func emitRowColorChanged(row int, color uint32) {
	hooks.PublishWithSource(hooks.EventRowColorChanged, hooks.RowColorPayload{
		Row: row, Color: color,
	}, hooks.CaptureSource(1))
}

// emitCustomWAVLoaded publishes EventCustomWAVLoaded when a user-supplied
// WAV becomes available as an instrument. isUpdate is true when the same
// instrument id was already present (re-upload replaces existing bytes).
func emitCustomWAVLoaded(instrumentID string, isUpdate bool) {
	hooks.PublishWithSource(hooks.EventCustomWAVLoaded, hooks.CustomWAVPayload{
		InstrumentID: instrumentID, IsUpdate: isUpdate,
	}, hooks.CaptureSource(1))
}

// emitInstrumentRenamed publishes EventInstrumentRenamed when an instrument
// id is rewritten by the user (rename closure in DrumView).
func emitInstrumentRenamed(oldID, newID string) {
	hooks.PublishWithSource(hooks.EventInstrumentRenamed, hooks.InstrumentRenamePayload{
		OldID: oldID, NewID: newID,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventInstrumentRenamed)
}

// emitSceneApplied publishes EventSceneApplied at the end of a successful
// RunScene call.
func emitSceneApplied(name string) {
	hooks.PublishWithSource(hooks.EventSceneApplied, hooks.ScenePayload{Name: name}, hooks.CaptureSource(1))
}

// emitUIStateApplied publishes EventUIStateApplied after a UI-state JSON
// file has been parsed and applied.
func emitUIStateApplied(path string) {
	hooks.PublishWithSource(hooks.EventUIStateApplied, hooks.UIStatePayload{Path: path}, hooks.CaptureSource(1))
}

// emitFavoriteToggled publishes EventFavoriteToggled when the user adds or
// removes an instrument from their favorites set.
func emitFavoriteToggled(instrumentID string, isFavorite bool) {
	hooks.PublishWithSource(hooks.EventFavoriteToggled, hooks.FavoritePayload{
		InstrumentID: instrumentID, IsFavorite: isFavorite,
	}, hooks.CaptureSource(1))
}

// emitLanguageChanged publishes EventLanguageChanged from the settings overlay
// language pick. old/new are locale strings ("en", "es").
func emitLanguageChanged(oldLoc, newLoc string) {
	hooks.PublishWithSource(hooks.EventLanguageChanged, hooks.LanguagePayload{
		Old: oldLoc, New: newLoc,
	}, hooks.CaptureSource(1))
}

// emitEQBandChange publishes EventEQBandChange when a per-channel EQ band
// gain changes. channel is the row instrument id ("kick", "snare", "main"…),
// band is the 0-indexed band number, gainDB is the new gain in decibels.
func emitEQBandChange(channel string, band int, gainDB float64) {
	hooks.PublishWithSource(hooks.EventEQBandChange, hooks.EQBandPayload{
		Channel: channel, Band: band, GainDB: gainDB,
	}, hooks.CaptureSource(1))
}

// emitEQFilterToggled publishes EventEQFilterToggled when a channel's HPF or
// LPF enable toggles. channel is the row instrument id ("main", "kick"…),
// filter is "hpf" or "lpf". Paired with recordUndoStep at the commit site
// (the toggle handler), mirroring emitEQBandChange / commitEQBand.
func emitEQFilterToggled(channel, filter string, enabled bool, cutoffHz float64) {
	hooks.PublishWithSource(hooks.EventEQFilterToggled, hooks.EQFilterPayload{
		Channel: channel, Filter: filter, Enabled: enabled, CutoffHz: cutoffHz,
	}, hooks.CaptureSource(1))
}

// emitInsertEffectAdded publishes EventInsertEffectAdded when an insert
// effect is added to a channel. effectType is the kind of effect
// (e.g. "delay", "reverb", "chorus").
func emitInsertEffectAdded(channel string, slot int, effectType string) {
	hooks.PublishWithSource(hooks.EventInsertEffectAdded, hooks.InsertEffectPayload{
		Channel: channel, Slot: slot, Type: effectType,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventInsertEffectAdded)
}

// emitInsertEffectRemoved publishes EventInsertEffectRemoved when an insert
// effect is removed from a channel.
func emitInsertEffectRemoved(channel string, slot int) {
	hooks.PublishWithSource(hooks.EventInsertEffectRemoved, hooks.InsertEffectPayload{
		Channel: channel, Slot: slot,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventInsertEffectRemoved)
}

// emitInsertEffectParam publishes EventInsertEffectParam when an insert
// effect parameter changes (e.g. delay time, reverb mix).
func emitInsertEffectParam(channel string, slot int, param string, value float64) {
	hooks.PublishWithSource(hooks.EventInsertEffectParam, hooks.InsertEffectPayload{
		Channel: channel, Slot: slot, Param: param, Value: value,
	}, hooks.CaptureSource(1))
}

// emitRowVolume publishes EventRowVolume once at slider release.
func emitRowVolume(row int, vol float64) {
	hooks.PublishWithSource(hooks.EventRowVolume, hooks.RowChangePayload{Row: row, Volume: vol}, hooks.CaptureSource(1))
}

// emitRowPan publishes EventRowPan once at slider release.
func emitRowPan(row int, pan float64) {
	hooks.PublishWithSource(hooks.EventRowPan, hooks.RowChangePayload{Row: row, Pan: pan}, hooks.CaptureSource(1))
}

// emitInsertEffectMoved publishes EventInsertEffectMoved on reorder.
func emitInsertEffectMoved(channel string, from, to int) {
	hooks.PublishWithSource(hooks.EventInsertEffectMoved, hooks.InsertEffectPayload{Channel: channel, FromSlot: from, ToSlot: to}, hooks.CaptureSource(1))
}

// emitInsertEffectToggled publishes EventInsertEffectToggled on enable/disable.
func emitInsertEffectToggled(channel string, slot int, enabled bool) {
	hooks.PublishWithSource(hooks.EventInsertEffectToggled, hooks.InsertEffectPayload{Channel: channel, Slot: slot, Enabled: enabled}, hooks.CaptureSource(1))
}

// emitSendChanged publishes EventSendChanged once at slider release. kind is
// "delay" or "reverb".
func emitSendChanged(channel, kind string, value float64) {
	hooks.PublishWithSource(hooks.EventSendChanged, hooks.SendPayload{Channel: channel, Kind: kind, Value: value}, hooks.CaptureSource(1))
}

// emitInstrumentParamsCommitted publishes EventInstrumentParamsCommitted once
// at knob release (the per-frame edit stays EventInstrumentParamChanged).
func emitInstrumentParamsCommitted(channel, recipe string) {
	hooks.PublishWithSource(hooks.EventInstrumentParamsCommitted, hooks.InstrumentParamPayload{Channel: channel, Recipe: recipe}, hooks.CaptureSource(1))
}

// emitInstrumentParamsReset publishes EventInstrumentParamsReset.
func emitInstrumentParamsReset(channel, recipe string) {
	hooks.PublishWithSource(hooks.EventInstrumentParamsReset, hooks.InstrumentParamPayload{Channel: channel, Recipe: recipe}, hooks.CaptureSource(1))
}

// emitAudioPanelStateChanged publishes EventAudioPanelStateChanged (userpref
// coverage only — not undoable).
func emitAudioPanelStateChanged(field string) {
	hooks.PublishWithSource(hooks.EventAudioPanelStateChanged, hooks.AudioPanelStatePayload{Field: field}, hooks.CaptureSource(1))
}

// emitCameraPan publishes EventCameraPan once at pan-gesture release with the
// cumulative screen-space delta. No longer verbose — shows in default narrative.
func emitCameraPan(dx, dy float64) {
	hooks.PublishWithSource(hooks.EventCameraPan, hooks.CameraPanPayload{DX: dx, DY: dy}, hooks.CaptureSource(1))
}

// emitCameraZoom publishes EventCameraZoom per zoom tick; the eventlogger
// coalescer collapses a continuous wheel/pinch burst into one trailing line.
func emitCameraZoom(factor float64) {
	hooks.PublishWithSource(hooks.EventCameraZoom, hooks.CameraZoomPayload{Factor: factor}, hooks.CaptureSource(1))
}

// emitUndo / emitRedo publish the undo/redo meta-action with the reverted
// edit's label (captured pre-Undo in undo_seam.go).
func emitUndo(label string) {
	hooks.PublishWithSource(hooks.EventUndo, hooks.UndoPayload{Label: label}, hooks.CaptureSource(1))
}
func emitRedo(label string) {
	hooks.PublishWithSource(hooks.EventRedo, hooks.UndoPayload{Label: label}, hooks.CaptureSource(1))
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
