package ui

import "github.com/ingyamilmolinar/beatmo/internal/i18n"

// undoActionLabelKeys maps each undoable action's English label (the
// hooks.ActionRegistry Label stored verbatim in the undo step) to its i18n key,
// so undo/redo notifications are fully localized — both the "Undo:"/"Redo:"
// prefix AND the action name. TestEveryUndoableActionHasLocalizedLabel asserts
// every undoable hooks.Kind's label has an entry here, so a new undoable action
// forces a translation rather than silently falling back to English.
var undoActionLabelKeys = map[string]i18n.Key{
	"change BPM":         i18n.KeyActionChangeBPM,
	"change subdivision": i18n.KeyActionChangeSubdivision,
	"add node":           i18n.KeyActionAddNode,
	"delete node":        i18n.KeyActionDeleteNode,
	"move node":          i18n.KeyActionMoveNode,
	"change node type":   i18n.KeyActionChangeNodeType,
	"edit node":          i18n.KeyActionEditNode,
	"set start node":     i18n.KeyActionSetStartNode,
	"add edge":           i18n.KeyActionAddEdge,
	"delete edge":        i18n.KeyActionDeleteEdge,
	"add row":            i18n.KeyActionAddRow,
	"delete row":         i18n.KeyActionDeleteRow,
	"change instrument":  i18n.KeyActionChangeInstrument,
	"set master volume":  i18n.KeyActionSetMasterVolume,
	"adjust EQ":          i18n.KeyActionAdjustEQ,
	"add effect":         i18n.KeyActionAddEffect,
	"remove effect":      i18n.KeyActionRemoveEffect,
	"adjust effect":      i18n.KeyActionAdjustEffect,
	"set row volume":     i18n.KeyActionSetRowVolume,
	"reorder effect":     i18n.KeyActionReorderEffect,
	"toggle effect":      i18n.KeyActionToggleEffect,
	"edit synth":         i18n.KeyActionEditSynth,
	"reset synth":        i18n.KeyActionResetSynth,
	"recolor row":        i18n.KeyActionRecolorRow,
	"toggle EQ filter":   i18n.KeyActionToggleEQFilter,
	"rename instrument":  i18n.KeyActionRenameInstrument,
	"edit sample":        i18n.KeyActionEditSample,
}

// notifyUndoRedo raises a localized undo/redo notification. prefixKey is
// KeyNotifUndo or KeyNotifRedo; englishLabel is the action's English registry
// label. The action name is stored as an "@<i18n key>" arg so it retranslates
// with the locale (display() resolves the nested key); an unmapped label falls
// back to the literal English label so the message is never empty.
func (dv *DrumView) notifyUndoRedo(prefixKey i18n.Key, englishLabel string) {
	if k, ok := undoActionLabelKeys[englishLabel]; ok {
		dv.notifyInfoKey(prefixKey, "@"+string(k))
		return
	}
	dv.notifyInfoKey(prefixKey, englishLabel)
}

// notifyUndoRedoDetail raises a localized undo/redo notification that names the
// changed parameter and its from → to values (e.g. "Undo: Volume 100% → 90%").
// prefixKey is KeyNotifUndoDetail / KeyNotifRedoDetail. The parameter label may
// be a literal ("BPM", a synth-param key) or an "@<i18n key>" reference that
// display() localizes; the values are locale-neutral (%, st, x). Used by
// performUndo/performRedo when the snapshot diff resolves to one scalar change.
func (dv *DrumView) notifyUndoRedoDetail(prefixKey i18n.Key, d changeDetail) {
	dv.notifyInfoKey(prefixKey, d.param, d.old, d.new)
}
