package hooks

// ActionScope classifies a user-action Kind by the kind of state it mutates,
// which determines whether it participates in the snapshot-journal undo system.
type ActionScope int

const (
	// ScopeDocument mutates the EXPORTED project document. Undoable unless
	// ExcludedReason is set (a known gap pending a real UI commit site / wiring).
	ScopeDocument ActionScope = iota
	// ScopeSession mutates runtime audio/UI state NOT in the export schema
	// (mute/solo, audio-panel prefs, user-asset loads). A snapshot can't
	// restore it, so it is never undoable.
	ScopeSession
	// ScopeTransport is playback control (play/stop/seek/pause/resume).
	ScopeTransport
	// ScopeProjectIO replaces or reads the whole document (import/export/scene).
	// Undo handles these via OnExternalLoad, not the per-action journal.
	ScopeProjectIO
	// ScopeRecording is the audio recording lifecycle.
	ScopeRecording
	// ScopeVerbose is high-frequency telemetry (camera/drag/per-frame param),
	// never an undo action.
	ScopeVerbose
	// ScopeInteraction is pure interaction telemetry (button tap, popup
	// open/close, scroll, search, text commit, view switch). Never undoable;
	// not document or session state — it records what the user touched, not a
	// state change.
	ScopeInteraction
)

// ActionMeta is one row of the canonical user-action registry: the single
// source of truth for a Kind's human label, scope, and undo classification.
type ActionMeta struct {
	Kind           Kind
	Label          string // human label, e.g. "add node"
	Tag            string // INFO bracket tag (lowercase), e.g. "node"
	Scope          ActionScope
	ExcludedReason string // non-empty ⇒ ScopeDocument but deliberately not undoable yet
}

// ActionRegistry has exactly one entry per Kind in KindAll. It is the single
// place that names and classifies every state-mutating user action. The undo
// recorded-set (internal/ui documentScopeKinds) is derived from this registry's
// Undoable() entries — the single source of truth; do not hand-maintain a
// divergent list. The derivation is pinned by TestUndoableSetMatchesRegistry
// (internal/ui); registry completeness by TestEveryKindClassified (this package).
//
// Labels for undoable (ScopeDocument, no ExcludedReason) entries are the
// user-facing undo labels and must stay stable — TestUndoableSetMatchesRegistry
// asserts they equal the derived documentScopeKinds.
var ActionRegistry = []ActionMeta{
	{EventPlayStart, "start playback", "play", ScopeTransport, ""},
	{EventPlayStop, "stop playback", "play", ScopeTransport, ""},
	{EventPaused, "pause", "play", ScopeTransport, ""},
	{EventResumed, "resume", "play", ScopeTransport, ""},
	{EventSeek, "seek", "seek", ScopeTransport, ""},

	{EventRecordStart, "start recording", "record", ScopeRecording, ""},
	{EventRecordStop, "stop recording", "record", ScopeRecording, ""},
	{EventRecordDropped, "recording dropped", "record", ScopeRecording, ""},

	{EventBPMChange, "change BPM", "bpm", ScopeDocument, ""},
	{EventSubdivChange, "change subdivision", "transport", ScopeDocument, ""},
	{EventLengthChange, "change length", "transport", ScopeSession, ""},

	{EventImport, "import project", "import", ScopeProjectIO, ""},
	{EventExport, "export project", "export", ScopeProjectIO, ""},

	{EventNodeAdded, "add node", "node", ScopeDocument, ""},
	{EventNodeDeleted, "delete node", "node", ScopeDocument, ""},
	{EventNodeMoved, "move node", "node", ScopeDocument, ""},
	{EventNodeTypeChanged, "change node type", "node", ScopeDocument, ""},
	{EventNodeParamsChanged, "edit node", "node", ScopeDocument, ""},
	{EventStartNodeChanged, "set start node", "node", ScopeDocument, ""},
	{EventEdgeAdded, "add edge", "edge", ScopeDocument, ""},
	{EventEdgeDeleted, "delete edge", "edge", ScopeDocument, ""},

	{EventRowAdded, "add row", "row", ScopeDocument, ""},
	{EventRowDeleted, "delete row", "row", ScopeDocument, ""},
	{EventRowInstrumentChange, "change instrument", "row", ScopeDocument, ""},
	{EventRowMute, "toggle mute", "row", ScopeSession, ""},
	{EventRowSolo, "toggle solo", "row", ScopeSession, ""},

	{EventMasterVolumeChange, "set master volume", "master", ScopeDocument, ""},
	{EventEQBandChange, "adjust EQ", "eq", ScopeDocument, ""},
	{EventInsertEffectAdded, "add effect", "fx", ScopeDocument, ""},
	{EventInsertEffectRemoved, "remove effect", "fx", ScopeDocument, ""},
	{EventInsertEffectParam, "adjust effect", "fx", ScopeDocument, ""},
	{EventRowVolume, "set row volume", "row", ScopeDocument, ""},
	{EventRowPan, "set row pan", "row", ScopeDocument, "import-only; no UI commit site yet"},
	{EventInsertEffectMoved, "reorder effect", "fx", ScopeDocument, ""},
	{EventInsertEffectToggled, "toggle effect", "fx", ScopeDocument, ""},
	{EventSendChanged, "adjust send", "send", ScopeDocument, "import-only; no UI commit site yet"},
	{EventInstrumentParamsCommitted, "edit synth", "synth", ScopeDocument, ""},
	{EventInstrumentParamsReset, "reset synth", "synth", ScopeDocument, ""},
	{EventAudioPanelStateChanged, "change audio-panel view", "panel", ScopeSession, ""},

	{EventRowColorChanged, "recolor row", "row", ScopeDocument, ""},
	{EventEQFilterToggled, "toggle EQ filter", "eq", ScopeDocument, ""},
	{EventCustomWAVLoaded, "load WAV", "sampler", ScopeSession, ""},
	{EventInstrumentRenamed, "rename instrument", "synth", ScopeDocument, ""},
	{EventSceneApplied, "apply scene", "scene", ScopeProjectIO, ""},
	{EventUIStateApplied, "apply UI state", "uistate", ScopeSession, ""},
	{EventFavoriteToggled, "toggle favorite", "favorite", ScopeSession, ""},
	{EventLanguageChanged, "change language", "lang", ScopeSession, ""},

	{EventRecipeSaved, "save recipe", "recipe", ScopeSession, ""},
	{EventRecipeCreated, "create recipe", "recipe", ScopeSession, ""},
	{EventRecipeDeleted, "delete recipe", "recipe", ScopeSession, ""},
	{EventKitApplied, "apply kit", "kit", ScopeSession, ""},

	{EventSampleSaved, "save sample", "sampler", ScopeSession, ""},
	{EventSampleCreated, "create sample", "sampler", ScopeSession, ""},
	{EventSampleReset, "reset sample", "sampler", ScopeSession, ""},
	{EventSampleEditChanged, "edit sample", "sampler", ScopeDocument, ""},

	{EventUndo, "undo", "undo", ScopeSession, ""},
	{EventRedo, "redo", "redo", ScopeSession, ""},

	{EventUITap, "tap", "ui", ScopeInteraction, ""},
	{EventPopupOpened, "open popup", "popup", ScopeInteraction, ""},
	{EventPopupClosed, "close popup", "popup", ScopeInteraction, ""},
	{EventScroll, "scroll", "scroll", ScopeInteraction, ""},
	{EventSearchChanged, "search", "search", ScopeInteraction, ""},
	{EventTextCommitted, "edit text", "input", ScopeInteraction, ""},
	{EventViewModeChanged, "switch view", "view", ScopeInteraction, ""},

	{EventCameraPan, "camera pan", "camera", ScopeVerbose, ""},
	{EventCameraZoom, "camera zoom", "camera", ScopeVerbose, ""},
	{EventDragProgress, "drag", "drag", ScopeVerbose, ""},
	{EventInstrumentParamChanged, "edit synth (live)", "synth", ScopeVerbose, ""},
}

// actionByKind indexes ActionRegistry for O(1) lookup. Built once at init.
var actionByKind = func() map[Kind]ActionMeta {
	m := make(map[Kind]ActionMeta, len(ActionRegistry))
	for _, a := range ActionRegistry {
		m[a.Kind] = a
	}
	return m
}()

// MetaFor returns the registry metadata for a Kind.
func MetaFor(k Kind) (ActionMeta, bool) {
	a, ok := actionByKind[k]
	return a, ok
}

// Undoable reports whether a Kind produces a snapshot-journal undo step:
// document-scope and not deliberately excluded.
func Undoable(k Kind) bool {
	a, ok := actionByKind[k]
	return ok && a.Scope == ScopeDocument && a.ExcludedReason == ""
}

// AllActions returns the full registry (read-only; do not mutate the slice).
func AllActions() []ActionMeta { return ActionRegistry }
