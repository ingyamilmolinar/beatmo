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
)

// ActionMeta is one row of the canonical user-action registry: the single
// source of truth for a Kind's human label, scope, and undo classification.
type ActionMeta struct {
	Kind           Kind
	Label          string // human label, e.g. "add node"
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
	// ── Transport ───────────────────────────────────────────────────────
	{EventPlayStart, "start playback", ScopeTransport, ""},
	{EventPlayStop, "stop playback", ScopeTransport, ""},
	{EventPaused, "pause", ScopeTransport, ""},
	{EventResumed, "resume", ScopeTransport, ""},
	{EventSeek, "seek", ScopeTransport, ""},

	// ── Recording ───────────────────────────────────────────────────────
	{EventRecordStart, "start recording", ScopeRecording, ""},
	{EventRecordStop, "stop recording", ScopeRecording, ""},
	{EventRecordDropped, "recording dropped", ScopeRecording, ""},

	// ── Time settings ───────────────────────────────────────────────────
	{EventBPMChange, "change BPM", ScopeDocument, ""},
	{EventSubdivChange, "change subdivision", ScopeDocument, ""},
	// Length is derived display state recomputed from the graph beat-path; it
	// is not in the export schema, so a snapshot cannot restore it independently.
	{EventLengthChange, "change length", ScopeSession, ""},

	// ── Project I/O ─────────────────────────────────────────────────────
	{EventImport, "import project", ScopeProjectIO, ""},
	{EventExport, "export project", ScopeProjectIO, ""},

	// ── Graph edits (all undoable) ──────────────────────────────────────
	{EventNodeAdded, "add node", ScopeDocument, ""},
	{EventNodeDeleted, "delete node", ScopeDocument, ""},
	{EventNodeMoved, "move node", ScopeDocument, ""},
	{EventNodeTypeChanged, "change node type", ScopeDocument, ""},
	{EventNodeParamsChanged, "edit node", ScopeDocument, ""},
	{EventStartNodeChanged, "set start node", ScopeDocument, ""},
	{EventEdgeAdded, "add edge", ScopeDocument, ""},
	{EventEdgeDeleted, "delete edge", ScopeDocument, ""},

	// ── Drum rows ───────────────────────────────────────────────────────
	{EventRowAdded, "add row", ScopeDocument, ""},
	{EventRowDeleted, "delete row", ScopeDocument, ""},
	{EventRowInstrumentChange, "change instrument", ScopeDocument, ""},
	// Mute/solo are session state (Row.Muted/Solo), not in exportBytes.
	{EventRowMute, "toggle mute", ScopeSession, ""},
	{EventRowSolo, "toggle solo", ScopeSession, ""},

	// ── Audio settings ──────────────────────────────────────────────────
	{EventMasterVolumeChange, "set master volume", ScopeDocument, ""},
	{EventEQBandChange, "adjust EQ", ScopeDocument, ""},
	{EventInsertEffectAdded, "add effect", ScopeDocument, ""},
	{EventInsertEffectRemoved, "remove effect", ScopeDocument, ""},
	{EventInsertEffectParam, "adjust effect", ScopeDocument, ""},
	{EventRowVolume, "set row volume", ScopeDocument, ""},
	// Row pan IS in the export schema, but no UI control writes it today (import
	// + display only). Re-classify as undoable when a commit site lands.
	{EventRowPan, "set row pan", ScopeDocument, "import-only; no UI commit site yet"},
	{EventInsertEffectMoved, "reorder effect", ScopeDocument, ""},
	{EventInsertEffectToggled, "toggle effect", ScopeDocument, ""},
	// Per-row delay/reverb sends ARE in the export schema, but no UI control
	// writes them today (import + display only).
	{EventSendChanged, "adjust send", ScopeDocument, "import-only; no UI commit site yet"},
	{EventInstrumentParamsCommitted, "edit synth", ScopeDocument, ""},
	{EventInstrumentParamsReset, "reset synth", ScopeDocument, ""},
	// Audio-panel prefs persist to userprefs, not the project document.
	{EventAudioPanelStateChanged, "change audio-panel view", ScopeSession, ""},

	// ── Round 2 narrative events ────────────────────────────────────────
	{EventRowColorChanged, "recolor row", ScopeDocument, ""},
	// Loading a user WAV is a library/asset op, not a journaled document edit.
	{EventCustomWAVLoaded, "load WAV", ScopeSession, ""},
	{EventInstrumentRenamed, "rename instrument", ScopeDocument, ""},
	{EventSceneApplied, "apply scene", ScopeProjectIO, ""},
	{EventUIStateApplied, "apply UI state", ScopeSession, ""},
	{EventFavoriteToggled, "toggle favorite", ScopeSession, ""},

	// ── Recipe / kit lifecycle (library ops, not journaled) ─────────────
	{EventRecipeSaved, "save recipe", ScopeSession, ""},
	{EventRecipeCreated, "create recipe", ScopeSession, ""},
	{EventRecipeDeleted, "delete recipe", ScopeSession, ""},
	{EventKitApplied, "apply kit", ScopeSession, ""},

	// ── Sampler lifecycle ───────────────────────────────────────────────
	{EventSampleSaved, "save sample", ScopeSession, ""},
	{EventSampleCreated, "create sample", ScopeSession, ""},
	{EventSampleReset, "reset sample", ScopeSession, ""},
	// The non-destructive sample-edit descriptor IS exported (sample_edit).
	{EventSampleEditChanged, "edit sample", ScopeDocument, ""},

	// ── Verbose (never undo actions) ────────────────────────────────────
	{EventCameraPan, "camera pan", ScopeVerbose, ""},
	{EventCameraZoom, "camera zoom", ScopeVerbose, ""},
	{EventDragProgress, "drag", ScopeVerbose, ""},
	{EventInstrumentParamChanged, "edit synth (live)", ScopeVerbose, ""},
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
