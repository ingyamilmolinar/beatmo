package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

var wantUndoableKinds = []hooks.Kind{
	hooks.EventNodeAdded, hooks.EventNodeDeleted, hooks.EventNodeMoved,
	hooks.EventNodeTypeChanged, hooks.EventNodeParamsChanged, hooks.EventStartNodeChanged,
	hooks.EventEdgeAdded, hooks.EventEdgeDeleted,
	hooks.EventRowAdded, hooks.EventRowDeleted, hooks.EventRowInstrumentChange,
	hooks.EventRowMute, hooks.EventRowSolo, hooks.EventRowColorChanged,
	hooks.EventRowVolume, hooks.EventRowPan, hooks.EventInstrumentRenamed,
	hooks.EventBPMChange, hooks.EventSubdivChange, hooks.EventLengthChange,
	hooks.EventMasterVolumeChange, hooks.EventEQBandChange,
	hooks.EventInsertEffectAdded, hooks.EventInsertEffectRemoved, hooks.EventInsertEffectParam,
	hooks.EventInsertEffectMoved, hooks.EventInsertEffectToggled, hooks.EventSendChanged,
	hooks.EventInstrumentParamsCommitted, hooks.EventInstrumentParamsReset,
	hooks.EventSampleEditChanged,
}

func TestUndoRecordedSetMatchesDeclared(t *testing.T) {
	want := map[hooks.Kind]bool{}
	for _, k := range wantUndoableKinds {
		want[k] = true
		if documentScopeKinds[k] == "" {
			t.Errorf("recorded kind %q missing a label in documentScopeKinds", k)
		}
	}
	for k := range documentScopeKinds {
		if !want[k] {
			t.Errorf("documentScopeKinds has %q not in wantUndoableKinds — deliberate? update the list", k)
		}
	}
}

func TestUndoRecordedSetExcludesNonDocument(t *testing.T) {
	excluded := []hooks.Kind{
		hooks.EventPlayStart, hooks.EventPlayStop, hooks.EventSeek,
		hooks.EventImport, hooks.EventExport, hooks.EventFavoriteToggled,
		hooks.EventAudioPanelStateChanged, hooks.EventInstrumentParamChanged,
		hooks.EventRecipeSaved, hooks.EventSceneApplied,
	}
	for _, k := range excluded {
		if _, ok := documentScopeKinds[k]; ok {
			t.Errorf("%q must NOT be in the undo recorded-set", k)
		}
	}
}
