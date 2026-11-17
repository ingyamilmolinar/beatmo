package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// All helpers in this file capture their caller's source via
// hooks.CaptureSource(1) and publish through hooks.PublishWithSource so
// subscribers (eventlogger, eventstream) receive the originating user-code
// site rather than this file. skip=1 means "my caller" — the user-action
// handler that invoked the helper.
//
// If a helper is itself wrapped by another helper, the outer helper must
// either inline the captureSource(1) call OR bump skip explicitly. Every
// helper here is called directly from the user-action site (skip=1) EXCEPT
// emitGridModeArmed/emitGridModeCanceled, which are called from
// gridModeManager.Enter/Cancel and therefore use skip=2 to attribute the
// event to the manager's caller.

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
		Type: t,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodeAdded)
}

// emitNodeDeleted publishes EventNodeDeleted with the deleted node's
// last-known coordinates. Called from deleteNodeInternal after the
// graph mutation completes.
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

// emitNodeTypeChanged publishes EventNodeTypeChanged when a node's type
// is mutated (Regular ↔ Silent ↔ Mute ↔ Invisible).
func emitNodeTypeChanged(id model.NodeID, oldT, newT model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodeTypeChanged, hooks.NodeTypePayload{
		ID:      int(id),
		OldType: oldT,
		NewType: newT,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodeTypeChanged)
}

// emitNodeCopied publishes EventNodeCopied. Interaction-scoped (not undoable):
// copy does not mutate the document, so it records no undo step.
func emitNodeCopied(id model.NodeID, i, j int, t model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodeCopied, hooks.NodeEdit{
		ID: int(id), I: i, J: j, Type: t,
	}, hooks.CaptureSource(1))
}

// emitNodeCut publishes EventNodeCut (narrative only). The undoable mutation is
// the subsequent node delete, which records its own undo step, so cut records
// none here.
func emitNodeCut(id model.NodeID, i, j int, t model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodeCut, hooks.NodeEdit{
		ID: int(id), I: i, J: j, Type: t,
	}, hooks.CaptureSource(1))
}

// emitNodePasted publishes EventNodePasted and records an undo step (paste is a
// document mutation).
func emitNodePasted(id model.NodeID, i, j int, t model.NodeType) {
	hooks.PublishWithSource(hooks.EventNodePasted, hooks.NodeEdit{
		ID: int(id), I: i, J: j, Type: t,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventNodePasted)
}

// emitNodeParamsChanged publishes EventNodeParamsChanged with the full
// new parameter snapshot. Subscribers can diff against their own copy
// to identify which field changed.
func emitNodeParamsChanged(id model.NodeID, p model.NodeParams) {
	emitNodeParams(id, p, "")
}

// emitNodeChordChanged is emitNodeParamsChanged for a chord-builder edit:
// the log line names the chord ("node id=3 chord C#m7/E").
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

// emitSeek publishes EventSeek with the destination position and its origin.
func emitSeek(beats, steps int, origin hooks.SeekOrigin) {
	hooks.PublishWithSource(hooks.EventSeek, hooks.SeekPayload{Beats: beats, Steps: steps, Origin: origin}, hooks.CaptureSource(1))
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

// emitRowInstrumentChange publishes EventRowInstrumentChange. oldInstrument
// is the instrument id the row was set to before the change.
func emitRowInstrumentChange(row int, oldInst, inst instrument.ID, name string) {
	hooks.PublishWithSource(hooks.EventRowInstrumentChange, hooks.RowChangePayload{
		Row:           row,
		OldInstrument: oldInst,
		Instrument:    inst,
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
func emitCustomWAVLoaded(instrumentID instrument.ID, isUpdate bool) {
	hooks.PublishWithSource(hooks.EventCustomWAVLoaded, hooks.CustomWAVPayload{
		InstrumentID: instrumentID, IsUpdate: isUpdate,
	}, hooks.CaptureSource(1))
}

// emitInstrumentRenamed publishes EventInstrumentRenamed when an instrument
// id is rewritten by the user (rename closure in DrumView).
func emitInstrumentRenamed(oldID, newID instrument.ID) {
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
func emitFavoriteToggled(instrumentID instrument.ID, isFavorite bool) {
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
func emitEQBandChange(ch channel.ID, band int, gainDB float64) {
	hooks.PublishWithSource(hooks.EventEQBandChange, hooks.EQBandPayload{
		Channel: ch, Band: band, GainDB: gainDB,
	}, hooks.CaptureSource(1))
}

// emitEQFilterToggled publishes EventEQFilterToggled when a channel's HPF or
// LPF enable toggles on channel ch;
// filter is "hpf" or "lpf". Paired with recordUndoStep at the commit site
// (the toggle handler), mirroring emitEQBandChange / commitEQBand.
func emitEQFilterToggled(ch channel.ID, filter hooks.EQFilter, enabled bool, cutoffHz float64) {
	hooks.PublishWithSource(hooks.EventEQFilterToggled, hooks.EQFilterPayload{
		Channel: ch, Filter: filter, Enabled: enabled, CutoffHz: cutoffHz,
	}, hooks.CaptureSource(1))
}

// emitInsertEffectAdded publishes EventInsertEffectAdded when an insert
// effect is added to a channel. effectType is the kind of effect
// (e.g. "delay", "reverb", "chorus").
func emitInsertEffectAdded(ch channel.ID, slot int, effectType string) {
	hooks.PublishWithSource(hooks.EventInsertEffectAdded, hooks.InsertEffectPayload{
		Channel: ch, Slot: slot, Type: effectType,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventInsertEffectAdded)
}

// emitInsertEffectRemoved publishes EventInsertEffectRemoved when an insert
// effect is removed from a channel.
func emitInsertEffectRemoved(ch channel.ID, slot int) {
	hooks.PublishWithSource(hooks.EventInsertEffectRemoved, hooks.InsertEffectPayload{
		Channel: ch, Slot: slot,
	}, hooks.CaptureSource(1))
	recordUndo(hooks.EventInsertEffectRemoved)
}

// emitInsertEffectParam publishes EventInsertEffectParam when an insert
// effect parameter changes (e.g. delay time, reverb mix).
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

// emitInstrumentParamsCommitted publishes EventInstrumentParamsCommitted once
// at knob release (the per-frame edit stays EventInstrumentParamChanged).
func emitInstrumentParamsCommitted(inst, origin instrument.ID) {
	hooks.PublishWithSource(hooks.EventInstrumentParamsCommitted, hooks.InstrumentParamPayload{Instrument: inst, Origin: origin}, hooks.CaptureSource(1))
}

// emitInstrumentParamsReset publishes EventInstrumentParamsReset.
func emitInstrumentParamsReset(inst, origin instrument.ID) {
	hooks.PublishWithSource(hooks.EventInstrumentParamsReset, hooks.InstrumentParamPayload{Instrument: inst, Origin: origin}, hooks.CaptureSource(1))
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

// emitGridModeArmed / emitGridModeCanceled publish the armed grid-mode
// lifecycle. Called ONLY from gridModeManager.Enter/Cancel — skip=2 attributes
// the event to the manager's caller (the user-action site: popup activation,
// sidebar button, tap handler).
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

// emitWarmupStarted publishes EventWarmupStarted when the playback readiness
// gate begins warming a circuit's working set.
//
// Unlike every other emit* helper here, src is passed in rather than captured
// via hooks.CaptureSource: finish()/onBegin can run on the warmup feeder
// goroutine or a frame tick, arbitrarily far from the action that actually
// triggered the warm-up (import, BPM change, ...). warmupController captures
// that source ONCE, in Begin(), and carries it through — see warmup_controller.go's
// src field doc comment. A fixed CaptureSource skip here would be wrong for
// at least three of the four call paths.
func emitWarmupStarted(src hooks.Source, total, cached int) {
	hooks.PublishWithSource(hooks.EventWarmupStarted, hooks.WarmupPayload{
		Total:  total,
		Cached: cached,
	}, src)
}

// emitWarmupComplete publishes EventWarmupComplete when the gate opens —
// either because the set finished warming or because the deadline elapsed.
// Not undoable: warm-up is a cache side-effect, not a document mutation.
// See emitWarmupStarted's comment for why src is passed in, not captured.
func emitWarmupComplete(src hooks.Source, p hooks.WarmupPayload) {
	hooks.PublishWithSource(hooks.EventWarmupComplete, p, src)
}

// emitThemeChanged publishes a profile-scope theme edit. scope is always
// "user" here; category/swatch may be empty (see hooks.ThemePayload).
func emitThemeChanged(scope hooks.ThemeScope, category, swatch string) {
	hooks.PublishWithSource(hooks.EventThemeChanged, hooks.ThemePayload{Scope: scope, Category: category, Swatch: swatch}, hooks.CaptureSource(1))
}

// emitThemeOverrideChanged publishes the "always use my theme" toggle.
func emitThemeOverrideChanged(on bool) {
	hooks.PublishWithSource(hooks.EventThemeChanged, hooks.ThemePayload{Scope: hooks.ThemeScopeUser, Override: &on}, hooks.CaptureSource(1))
}

// emitCircuitThemeChanged publishes a circuit-scope edit AND records the undo
// step (document scope: the snapshot journal captures the export bytes,
// which carry the theme block).
func emitCircuitThemeChanged(category, swatch string) {
	hooks.PublishWithSource(hooks.EventCircuitThemeChanged, hooks.ThemePayload{Scope: hooks.ThemeScopeCircuit, Category: category, Swatch: swatch}, hooks.CaptureSource(1))
	recordUndo(hooks.EventCircuitThemeChanged)
}

// emitSampleEditChanged publishes EventSampleEditChanged once per completed
// Sampler edit gesture. The descriptor itself is pushed to audio LIVE, per value
// change (audio.ApplySampleEditLive); this is the gesture-level announcement, so
// a spin of the Pitch wheel yields one narrative line rather than one per detent.
func emitSampleEditChanged(sampleID instrument.ID) {
	hooks.PublishWithSource(hooks.EventSampleEditChanged, hooks.SamplePayload{SampleID: sampleID}, hooks.CaptureSource(1))
}
