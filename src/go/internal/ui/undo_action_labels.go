package ui

import "github.com/ingyamilmolinar/beatmo/internal/i18n"

// Keyed by the registry's English label stored in the step, so both prefix and action name localize; a new
// undoable action without an entry fails TestEveryUndoableActionHasLocalizedLabel.
var undoActionLabelKeys = map[string]i18n.Key{
	"change BPM":           i18n.KeyActionChangeBPM,
	"change subdivision":   i18n.KeyActionChangeSubdivision,
	"add node":             i18n.KeyActionAddNode,
	"delete node":          i18n.KeyActionDeleteNode,
	"paste node":           i18n.KeyActionPasteNode,
	"replace node":         i18n.KeyActionReplaceNode,
	"move node":            i18n.KeyActionMoveNode,
	"change node type":     i18n.KeyActionChangeNodeType,
	"edit node":            i18n.KeyActionEditNode,
	"set start node":       i18n.KeyActionSetStartNode,
	"add edge":             i18n.KeyActionAddEdge,
	"delete edge":          i18n.KeyActionDeleteEdge,
	"add row":              i18n.KeyActionAddRow,
	"delete row":           i18n.KeyActionDeleteRow,
	"change instrument":    i18n.KeyActionChangeInstrument,
	"set master volume":    i18n.KeyActionSetMasterVolume,
	"adjust EQ":            i18n.KeyActionAdjustEQ,
	"add effect":           i18n.KeyActionAddEffect,
	"remove effect":        i18n.KeyActionRemoveEffect,
	"adjust effect":        i18n.KeyActionAdjustEffect,
	"set row volume":       i18n.KeyActionSetRowVolume,
	"reorder effect":       i18n.KeyActionReorderEffect,
	"toggle effect":        i18n.KeyActionToggleEffect,
	"edit synth":           i18n.KeyActionEditSynth,
	"reset synth":          i18n.KeyActionResetSynth,
	"recolor row":          i18n.KeyActionRecolorRow,
	"change circuit theme": i18n.KeyActionChangeCircuitTheme,
	"toggle EQ filter":     i18n.KeyActionToggleEQFilter,
	"rename instrument":    i18n.KeyActionRenameInstrument,
	"edit sample":          i18n.KeyActionEditSample,
	"create group":         i18n.KeyActionCreateGroup,
	"edit group":           i18n.KeyActionEditGroup,
	"delete group":         i18n.KeyActionDeleteGroup,
}

// The action name is an "@<key>" arg so it retranslates with the locale; an unmapped label falls back to English,
// never empty.
func (dv *DrumView) notifyUndoRedo(prefixKey i18n.Key, englishLabel string) {
	if k, ok := undoActionLabelKeys[englishLabel]; ok {
		dv.notifyInfoKey(prefixKey, "@"+string(k))
		return
	}
	dv.notifyInfoKey(prefixKey, englishLabel)
}

// "Undo: Volume 100% → 90%"; the label may be literal or "@<key>", the values are locale-neutral.
func (dv *DrumView) notifyUndoRedoDetail(prefixKey i18n.Key, d changeDetail) {
	dv.notifyInfoKey(prefixKey, d.param, d.old, d.new)
}
