package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Every helper captures its CALLER's source (skip=1) so subscribers see the user-action site, not this file;
// emitGridModeArmed/Canceled use skip=2 because gridModeManager calls them. A wrapping helper must bump skip.

func emitNodeAdded(n *uiNode, t model.NodeType) {
	if n == nil {
		return
	}
	hooks.PublishWithSource(hooks.EventNodeAdded, hooks.NodeEdit{
		ID:   int(n.ID),
		I:    n.I,
		J:    n.J,
		Type: t,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodeAdded)
}

func emitNodeDeleted(id model.NodeID, i, j int, t model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodeDeleted, hooks.NodeEdit{
		ID: int(id), I: i, J: j, Type: t,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodeDeleted)
}

// emitNodeMoved publishes EventNodeMoved on drag-end.
func emitNodeMoved(id model.NodeID, i, j int, t model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodeMoved, hooks.NodeEdit{
		ID: int(id), I: i, J: j, Type: t,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodeMoved)
}

func emitNodeTypeChanged(id model.NodeID, oldT, newT model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodeTypeChanged, hooks.NodeTypePayload{
		ID:      int(id),
		OldType: oldT,
		NewType: newT,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodeTypeChanged)
}

// Not undoable: copy does not mutate the document.
func emitNodeCopied(id model.NodeID, i, j int, t model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodeCopied, hooks.NodeEdit{
		ID: int(id), I: i, J: j, Type: t,
	}, hooks.CaptureSource(1))
}

// Narrative only: the delete that follows records the undo step.
func emitNodeCut(id model.NodeID, i, j int, t model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodeCut, hooks.NodeEdit{
		ID: int(id), I: i, J: j, Type: t,
	}, hooks.CaptureSource(1))
}

// Paste mutates the document, so it records an undo step.
func emitNodePasted(id model.NodeID, i, j int, t model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodePasted, hooks.NodeEdit{
		ID: int(id), I: i, J: j, Type: t,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodePasted)
}

// The full snapshot: subscribers diff against their own copy.
func emitNodeParamsChanged(id model.NodeID, p model.NodeParams) {
	emitNodeParams(id, p, "")
}

func emitNodeChordChanged(id model.NodeID, p model.NodeParams, chord string) {
	emitNodeParams(id, p, chord)
}

func emitNodeParams(id model.NodeID, p model.NodeParams, chord string) {
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
		Chord:      chord,
	}, hooks.CaptureSource(2))
	recordUndo(hooks.EventNodeParamsChanged)
}

// emitGroupCreated publishes EventGroupCreated and records the undo step.
func emitGroupCreated(id model.GroupID, name string, nodeIDs []model.NodeID) {
	nodes := make([]int, 0, len(nodeIDs))
	for _, n := range nodeIDs {
		nodes = append(nodes, int(n))
	}
	hooks.PublishWithSource(hooks.EventGroupCreated, hooks.GroupPayload{
		ID: int(id), Name: name, Nodes: nodes,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventGroupCreated)
}

// emitGroupChanged publishes EventGroupChanged (rename/membership/rules).
func emitGroupChanged(id model.GroupID) {
	hooks.PublishWithSource(hooks.EventGroupChanged, hooks.GroupPayload{ID: int(id)}, hooks.CaptureSource(1))
	recordUndo(hooks.EventGroupChanged)
}

// emitGroupDeleted publishes EventGroupDeleted.
func emitGroupDeleted(id model.GroupID) {
	hooks.PublishWithSource(hooks.EventGroupDeleted, hooks.GroupPayload{ID: int(id)}, hooks.CaptureSource(1))
	recordUndo(hooks.EventGroupDeleted)
}

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

// emitSeek publishes EventSeek with the destination position and its origin.
func emitSeek(beats, steps int, origin hooks.SeekOrigin) {
	hooks.PublishWithSource(hooks.EventSeek, hooks.SeekPayload{Beats: beats, Steps: steps, Origin: origin}, hooks.CaptureSource(1))
}

// emitSubdivChange publishes EventSubdivChange.
func emitSubdivChange(subdiv int) {
	hooks.PublishWithSource(hooks.EventSubdivChange, hooks.SubdivPayload{Subdiv: subdiv}, hooks.CaptureSource(1))
	recordUndo(hooks.EventSubdivChange)
}

// Not undoable: length is derived display state, not in the export schema.
func emitLengthChange(length int) {
	hooks.PublishWithSource(hooks.EventLengthChange, hooks.LengthPayload{Length: length}, hooks.CaptureSource(1))
}

// emitRowAdded publishes EventRowAdded.
func emitRowAdded(row int, inst instrument.ID, name string) {
	hooks.PublishWithSource(hooks.EventRowAdded, hooks.RowChangePayload{
		Row: row, Instrument: inst, Name: name,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventRowAdded)
}

// emitRowDeleted publishes EventRowDeleted.
func emitRowDeleted(row int) {
	hooks.PublishWithSource(hooks.EventRowDeleted, hooks.RowChangePayload{Row: row}, hooks.CaptureSource(1))
	recordUndo(hooks.EventRowDeleted)
}

func emitRowInstrumentChange(row int, oldInst, inst instrument.ID, name string) {
	hooks.PublishWithSource(hooks.EventRowInstrumentChange, hooks.RowChangePayload{
		Row:           row,
		OldInstrument: oldInst,
		Instrument:    inst,
		Name:          name,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventRowInstrumentChange)
}

// Not undoable: mute is session state, not in the export schema.
func emitRowMute(row int, mute bool) {
	hooks.PublishWithSource(hooks.EventRowMute, hooks.RowChangePayload{Row: row, Mute: mute}, hooks.CaptureSource(1))
}

// Not undoable: solo is session state, not in the export schema.
func emitRowSolo(row int, solo bool) {
	hooks.PublishWithSource(hooks.EventRowSolo, hooks.RowChangePayload{Row: row, Solo: solo}, hooks.CaptureSource(1))
}

func emitMasterVolumeChange(v float64) {
	hooks.PublishWithSource(hooks.EventMasterVolumeChange, hooks.MasterVolumePayload{Volume: v}, hooks.CaptureSource(1))
}

func emitBPMChange(bpm int) {
	hooks.PublishWithSource(hooks.EventBPMChange, hooks.BPMPayload{BPM: bpm}, hooks.CaptureSource(1))
	recordUndo(hooks.EventBPMChange)
}

func emitImport(bytes, nodes, rows int) {
	hooks.PublishWithSource(hooks.EventImport, hooks.ImportPayload{
		Bytes: bytes, Nodes: nodes, Rows: rows,
	}, hooks.CaptureSource(1))
}

// Coalesced: the wheel drag emits per frame and the trailing pick is the meaningful event.
func emitRowColorChanged(row int, color uint32) {
	hooks.PublishWithSource(hooks.EventRowColorChanged, hooks.RowColorPayload{
		Row: row, Color: color,
	}, hooks.CaptureSource(1))
}

// isUpdate when the id was already present (a re-upload replaces the bytes).
func emitCustomWAVLoaded(instrumentID instrument.ID, isUpdate bool) {
	hooks.PublishWithSource(hooks.EventCustomWAVLoaded, hooks.CustomWAVPayload{
		InstrumentID: instrumentID, IsUpdate: isUpdate,
	}, hooks.CaptureSource(1))
}

func emitInstrumentRenamed(oldID, newID instrument.ID) {
	hooks.PublishWithSource(hooks.EventInstrumentRenamed, hooks.InstrumentRenamePayload{
		OldID: oldID, NewID: newID,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventInstrumentRenamed)
}

func emitSceneApplied(name string) {
	hooks.PublishWithSource(hooks.EventSceneApplied, hooks.ScenePayload{Name: name}, hooks.CaptureSource(1))
}

func emitUIStateApplied(path string) {
	hooks.PublishWithSource(hooks.EventUIStateApplied, hooks.UIStatePayload{Path: path}, hooks.CaptureSource(1))
}

func emitFavoriteToggled(instrumentID instrument.ID, isFavorite bool) {
	hooks.PublishWithSource(hooks.EventFavoriteToggled, hooks.FavoritePayload{
		InstrumentID: instrumentID, IsFavorite: isFavorite,
	}, hooks.CaptureSource(1))
}

func emitLanguageChanged(oldLoc, newLoc string) {
	hooks.PublishWithSource(hooks.EventLanguageChanged, hooks.LanguagePayload{
		Old: oldLoc, New: newLoc,
	}, hooks.CaptureSource(1))
}

func emitEQBandChange(ch channel.ID, band int, gainDB float64) {
	hooks.PublishWithSource(hooks.EventEQBandChange, hooks.EQBandPayload{
		Channel: ch, Band: band, GainDB: gainDB,
	}, hooks.CaptureSource(1))
}

// Paired with recordUndoStep at the toggle's commit site.
func emitEQFilterToggled(ch channel.ID, filter hooks.EQFilter, enabled bool, cutoffHz float64) {
	hooks.PublishWithSource(hooks.EventEQFilterToggled, hooks.EQFilterPayload{
		Channel: ch, Filter: filter, Enabled: enabled, CutoffHz: cutoffHz,
	}, hooks.CaptureSource(1))
}

func emitInsertEffectAdded(ch channel.ID, slot int, effectType string) {
	hooks.PublishWithSource(hooks.EventInsertEffectAdded, hooks.InsertEffectPayload{
		Channel: ch, Slot: slot, Type: effectType,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventInsertEffectAdded)
}

func emitInsertEffectRemoved(ch channel.ID, slot int) {
	hooks.PublishWithSource(hooks.EventInsertEffectRemoved, hooks.InsertEffectPayload{
		Channel: ch, Slot: slot,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventInsertEffectRemoved)
}

func emitInsertEffectParam(ch channel.ID, slot int, param string, value float64) {
	hooks.PublishWithSource(hooks.EventInsertEffectParam, hooks.InsertEffectPayload{
		Channel: ch, Slot: slot, Param: param, Value: value,
	}, hooks.CaptureSource(1))
}

// emitRowVolume publishes EventRowVolume once at slider release.
func emitRowVolume(row int, vol float64) {
	hooks.PublishWithSource(hooks.EventRowVolume, hooks.RowChangePayload{Row: row, Volume: vol}, hooks.CaptureSource(1))
}

// emitInsertEffectMoved publishes EventInsertEffectMoved on reorder.
func emitInsertEffectMoved(ch channel.ID, from, to int) {
	hooks.PublishWithSource(hooks.EventInsertEffectMoved, hooks.InsertEffectPayload{Channel: ch, FromSlot: from, ToSlot: to}, hooks.CaptureSource(1))
}

// emitInsertEffectToggled publishes EventInsertEffectToggled on enable/disable.
func emitInsertEffectToggled(ch channel.ID, slot int, enabled bool) {
	hooks.PublishWithSource(hooks.EventInsertEffectToggled, hooks.InsertEffectPayload{Channel: ch, Slot: slot, Enabled: enabled}, hooks.CaptureSource(1))
}

// Once at release; the per-frame edit stays EventInstrumentParamChanged.
func emitInstrumentParamsCommitted(inst, origin instrument.ID) {
	hooks.PublishWithSource(hooks.EventInstrumentParamsCommitted, hooks.InstrumentParamPayload{Instrument: inst, Origin: origin}, hooks.CaptureSource(1))
}

// emitInstrumentParamsReset publishes EventInstrumentParamsReset.
func emitInstrumentParamsReset(inst, origin instrument.ID) {
	hooks.PublishWithSource(hooks.EventInstrumentParamsReset, hooks.InstrumentParamPayload{Instrument: inst, Origin: origin}, hooks.CaptureSource(1))
}

// Userpref coverage only, not undoable.
func emitAudioPanelStateChanged(field string) {
	hooks.PublishWithSource(hooks.EventAudioPanelStateChanged, hooks.AudioPanelStatePayload{Field: field}, hooks.CaptureSource(1))
}

// Once per pan gesture, at release.
func emitCameraPan(dx, dy float64) {
	hooks.PublishWithSource(hooks.EventCameraPan, hooks.CameraPanPayload{DX: dx, DY: dy}, hooks.CaptureSource(1))
}

// Per tick; the eventlogger coalescer collapses a burst into one line.
func emitCameraZoom(factor float64) {
	hooks.PublishWithSource(hooks.EventCameraZoom, hooks.CameraZoomPayload{Factor: factor}, hooks.CaptureSource(1))
}

// The label is the reverted edit's, captured before the undo.
func emitUndo(label string) {
	hooks.PublishWithSource(hooks.EventUndo, hooks.UndoPayload{Label: label}, hooks.CaptureSource(1))
}
func emitRedo(label string) {
	hooks.PublishWithSource(hooks.EventRedo, hooks.UndoPayload{Label: label}, hooks.CaptureSource(1))
}

// Called ONLY from gridModeManager.Enter/Cancel, hence skip=2: the event belongs to the manager's caller.
func emitGridModeArmed(mode string, id model.NodeID, i, j int) {
	hooks.PublishWithSource(hooks.EventGridModeArmed, hooks.GridModePayload{
		Mode: mode, ID: int(id), I: i, J: j,
	}, hooks.CaptureSource(2))
}

func emitGridModeCanceled(mode string, id model.NodeID, i, j int, reason string) {
	hooks.PublishWithSource(hooks.EventGridModeCanceled, hooks.GridModePayload{
		Mode: mode, ID: int(id), I: i, J: j, Reason: reason,
	}, hooks.CaptureSource(2))
}

// src is passed in, not captured: the gate can finish on the feeder goroutine or a frame tick, far from the
// action that triggered it, so warmupController captures it once in Begin().
func emitWarmupStarted(src hooks.Source, total, cached int) {
	hooks.PublishWithSource(hooks.EventWarmupStarted, hooks.WarmupPayload{
		Total:  total,
		Cached: cached,
	}, src)
}

// Not undoable: warm-up is a cache side effect. src is passed in for the same reason as emitWarmupStarted.
func emitWarmupComplete(src hooks.Source, p hooks.WarmupPayload) {
	hooks.PublishWithSource(hooks.EventWarmupComplete, p, src)
}

// scope is always "user" here; category/swatch may be empty.
func emitThemeChanged(scope hooks.ThemeScope, category, swatch string) {
	hooks.PublishWithSource(hooks.EventThemeChanged, hooks.ThemePayload{Scope: scope, Category: category, Swatch: swatch}, hooks.CaptureSource(1))
}

// emitThemeOverrideChanged publishes the "always use my theme" toggle.
func emitThemeOverrideChanged(on bool) {
	hooks.PublishWithSource(hooks.EventThemeChanged, hooks.ThemePayload{Scope: hooks.ThemeScopeUser, Override: &on}, hooks.CaptureSource(1))
}

// Document scope: the snapshot journal's export bytes carry the theme block, so it records an undo step.
func emitCircuitThemeChanged(category, swatch string) {
	hooks.PublishWithSource(hooks.EventCircuitThemeChanged, hooks.ThemePayload{Scope: hooks.ThemeScopeCircuit, Category: category, Swatch: swatch}, hooks.CaptureSource(1))
	recordUndo(hooks.EventCircuitThemeChanged)
}

// Once per gesture: the edit reaches audio live per value change, so a wheel spin yields one line, not one
// per detent.
func emitSampleEditChanged(sampleID instrument.ID) {
	hooks.PublishWithSource(hooks.EventSampleEditChanged, hooks.SamplePayload{SampleID: sampleID}, hooks.CaptureSource(1))
}
