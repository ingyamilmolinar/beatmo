// Package hooks provides a small, non-blocking event bus for user-action
// triggers. Producers publish typed events; subscribers run off-thread on
// a shared async worker pool so neither the audio thread nor the UI's
// Update loop ever stalls on a slow handler.
package hooks

import "time"

// Kind identifies a category of event.
type Kind string

const (
	// Playback lifecycle.
	EventPlayStart Kind = "play.start"
	EventPlayStop  Kind = "play.stop"
	EventPaused    Kind = "transport.pause"
	EventResumed   Kind = "transport.resume"
	EventSeek      Kind = "transport.seek"

	// Recording lifecycle.
	EventRecordStart   Kind = "record.start"
	EventRecordStop    Kind = "record.stop"
	EventRecordDropped Kind = "record.dropped" // capture backpressure

	// Transport / time settings.
	EventBPMChange    Kind = "bpm.change"
	EventSubdivChange Kind = "transport.subdiv"
	EventLengthChange Kind = "transport.length"

	// Project I/O.
	EventImport Kind = "import"
	EventExport Kind = "export"

	// Graph edits — discrete state changes only (verbose drag-progress
	// events live under the Verbose* kinds below).
	EventNodeAdded         Kind = "node.added"
	EventNodeDeleted       Kind = "node.deleted"
	EventNodeMoved         Kind = "node.moved"
	EventNodeTypeChanged   Kind = "node.type"
	EventNodeParamsChanged Kind = "node.params"
	EventStartNodeChanged  Kind = "node.start"
	EventEdgeAdded         Kind = "edge.added"
	EventEdgeDeleted       Kind = "edge.deleted"

	// Drum rows.
	EventRowAdded            Kind = "row.added"
	EventRowDeleted          Kind = "row.deleted"
	EventRowInstrumentChange Kind = "row.instrument"
	EventRowMute             Kind = "row.mute"
	EventRowSolo             Kind = "row.solo"

	// Audio settings (non-recording).
	EventMasterVolumeChange  Kind = "audio.master_volume"
	EventEQBandChange        Kind = "audio.eq_band"
	EventInsertEffectAdded   Kind = "audio.insert_added"
	EventInsertEffectRemoved Kind = "audio.insert_removed"
	EventInsertEffectParam   Kind = "audio.insert_param"

	// Audio settings — release-committed (added for full event coverage +
	// undo). These fire once per gesture at drag-release, never per-frame.
	EventRowVolume           Kind = "row.volume"
	EventRowPan              Kind = "row.pan"
	EventInsertEffectMoved   Kind = "audio.insert_moved"
	EventInsertEffectToggled Kind = "audio.insert_toggled"
	EventSendChanged         Kind = "audio.send"
	// Synth params committed once at knob-release (the per-frame edit stays
	// the verbose EventInstrumentParamChanged below). Reset clears overrides.
	EventInstrumentParamsCommitted Kind = "audio.synth_committed"
	EventInstrumentParamsReset     Kind = "audio.synth_reset"
	// Userpref coverage only (NOT undoable): audio-panel state persisted.
	EventAudioPanelStateChanged Kind = "uistate.audio_panel"

	// Synth recipe / instrument synthesis parameters. Fires when a user
	// edits an instrument's synth params via the instrument editor (or
	// any other path that mutates instrumentParamsMgr in internal/audio).
	//
	// Slider drag publishes this on every frame the slider value moves
	// (60Hz typical), matching the pattern of EventCameraPan / Zoom /
	// DragProgress below. The narrative INFO consumer filters it out
	// via IsVerbose so console-log throughput stays bounded; the JSONL
	// eventstream still records every event when verbose mode is on.
	EventInstrumentParamChanged Kind = "verbose.instrument_param"

	// Round 2 additions: user-narrative events with no prior home.
	EventRowColorChanged   Kind = "row.color"
	EventCustomWAVLoaded   Kind = "audio.wav_loaded"
	EventInstrumentRenamed Kind = "audio.instrument_renamed"
	EventSceneApplied      Kind = "scene.applied"
	EventUIStateApplied    Kind = "uistate.applied"
	EventFavoriteToggled   Kind = "favorite.toggled"

	// EventEQFilterToggled names the master/channel EQ HPF or LPF enable toggle.
	// Reserved name only: the UI toggle (eq_panel_zone.go OnToggleHPF/OnToggleLPF)
	// does not emit it yet. Classified ScopeDocument with an ExcludedReason in
	// the action registry until a commit site + undo tap are wired.
	EventEQFilterToggled Kind = "audio.eq_filter_toggled"

	// EventLanguageChanged fires when the user switches UI language in the
	// settings overlay. ScopeSession (locale persists to userprefs, not the
	// project document).
	EventLanguageChanged Kind = "lang.changed"

	// EventUndo / EventRedo fire when the user reverts / re-applies one edit.
	// ScopeSession (the meta-action itself is never journaled).
	EventUndo Kind = "undo"
	EventRedo Kind = "redo"

	// EventUITap fires once on a button/pill/toggle/keycap press edge (Layer-B
	// interaction telemetry). The domain effect, if any, is a separate event.
	EventUITap Kind = "ui.tap"

	// EventPopupOpened / EventPopupClosed fire from the shared overlay portal
	// for every menu/dialog/picker/slider-popup. Reason on close is
	// "explicit", "top", or "dismiss" (Esc/click-outside self-close).
	EventPopupOpened Kind = "popup.opened"
	EventPopupClosed Kind = "popup.closed"

	// EventScroll fires at scroll-gesture end (touch lift / scrollbar drag
	// release / stepped-grid drag end). Coalesced so a flick + momentum is one
	// line. Surface names the scrolled region.
	EventScroll Kind = "ui.scroll"

	// EventSearchChanged fires when a search field's query changes. Emitted
	// per-keystroke at the source but coalesced so rapid typing → one trailing
	// line with the settled query. Surface names the search context (e.g.
	// "inst-menu"); Query is the current input value ("" means cleared).
	EventSearchChanged Kind = "ui.search"

	// EventTextCommitted fires when a generic text field commits (Enter/blur).
	// Domain-specific text commits (instrument rename, BPM entry) use their own
	// events and do NOT emit this.
	EventTextCommitted Kind = "ui.text"

	// EventViewModeChanged fires when the mobile bottom-nav switches the
	// visible view (pads/eq/wave/spectrum/levels/chain/synth/sampler).
	EventViewModeChanged Kind = "ui.view"

	// Phase 4 (synth-recipe refactor): user-narrative events for recipe
	// save / clone / delete and kit apply. RecipePayload carries the
	// recipe id + base recipe id (for clones) + the instrument id the
	// action originated from. EventKitApplied uses KitPayload; the kit
	// data model lands in Phase 6 (audio.Kit) but the event kind is
	// reserved now so eventlogger coverage and NumNonVerbose stay stable
	// across the rollout.
	EventRecipeSaved   Kind = "audio.recipe_saved"
	EventRecipeCreated Kind = "audio.recipe_created"
	EventRecipeDeleted Kind = "audio.recipe_deleted"
	EventKitApplied    Kind = "audio.kit_applied"

	// Sampler-tab lifecycle: SampleSaved fires when a baked sample overrides
	// its source instrument in place; SampleCreated fires on Save As (a new
	// embedded instrument). Both carry SamplePayload.
	EventSampleSaved   Kind = "audio.sample_saved"
	EventSampleCreated Kind = "audio.sample_created"
	// EventSampleReset fires when an instrument is reverted to its factory
	// state via the Sampler/Synth Reset. SamplePayload carries SampleID (the
	// instrument) and SourceID (the recipe it reverted to, or "" for a user
	// WAV restored to its original buffer) — enough to invert in a future
	// undo/redo journal.
	EventSampleReset Kind = "audio.sample_reset"
	// EventSampleEditChanged fires when an instrument's non-destructive
	// sample-edit descriptor is set or cleared (Sampler-tab Save on a synth
	// source, Reset, import, or startup rehydration). SamplePayload carries
	// SampleID (the instrument). The instrument stays a synth — the edit is
	// applied to the freshly-rendered recipe buffer at trigger time.
	EventSampleEditChanged Kind = "audio.sample_edit"

	// Camera events — promoted out of verbose. EventCameraPan fires once per
	// pan gesture (at release, carrying the cumulative delta) via the
	// cameraGesture tracker in internal/ui. EventCameraZoom fires per zoom
	// tick; the eventlogger coalescer debounce-trails a wheel/pinch burst into
	// one trailing line. Both now appear in the default INFO narrative.
	EventCameraPan  Kind = "verbose.pan"
	EventCameraZoom Kind = "verbose.zoom"

	// Verbose / opt-in (only delivered when verbose mode is enabled on the
	// bus or the eventstream sink). These fire many times per frame; the
	// default sink filters them out.
	EventDragProgress Kind = "verbose.drag"
)

// KindAll lists every Kind so eventstream sinks (and tests) can iterate
// over the universe of events without enumerating them individually.
// Keep in sync with the constants above. Verbose kinds live at the tail
// so subscribers can split-subscribe by slicing.
var KindAll = []Kind{
	EventPlayStart, EventPlayStop, EventPaused, EventResumed, EventSeek,
	EventRecordStart, EventRecordStop, EventRecordDropped,
	EventBPMChange, EventSubdivChange, EventLengthChange,
	EventImport, EventExport,
	EventNodeAdded, EventNodeDeleted, EventNodeMoved, EventNodeTypeChanged,
	EventNodeParamsChanged, EventStartNodeChanged,
	EventEdgeAdded, EventEdgeDeleted,
	EventRowAdded, EventRowDeleted, EventRowInstrumentChange, EventRowMute, EventRowSolo,
	EventMasterVolumeChange, EventEQBandChange,
	EventInsertEffectAdded, EventInsertEffectRemoved, EventInsertEffectParam,
	EventRowVolume, EventRowPan,
	EventInsertEffectMoved, EventInsertEffectToggled, EventSendChanged,
	EventInstrumentParamsCommitted, EventInstrumentParamsReset,
	EventAudioPanelStateChanged,
	EventRowColorChanged, EventCustomWAVLoaded, EventInstrumentRenamed,
	EventEQFilterToggled,
	EventSceneApplied, EventUIStateApplied, EventFavoriteToggled,
	EventLanguageChanged,
	EventRecipeSaved, EventRecipeCreated, EventRecipeDeleted, EventKitApplied,
	EventSampleSaved, EventSampleCreated, EventSampleReset, EventSampleEditChanged,
	EventUndo, EventRedo,
	EventUITap,
	EventPopupOpened, EventPopupClosed,
	EventScroll,
	EventSearchChanged,
	EventTextCommitted,
	EventViewModeChanged,
	EventCameraPan, EventCameraZoom, EventDragProgress, EventInstrumentParamChanged,
}

// numNonVerbose is the count of non-verbose kinds in KindAll.
// KindAll[:NumNonVerbose] excludes the high-frequency verbose events.
// Pre-Phase-4 the constant was 36 but the actual non-verbose tail was
// already 37 (off-by-one drift across earlier bumps). Phase 4 added 4
// (recipe save/create/delete + kit apply) and corrected the count to
// match KindAll: 37 + 4 = 41. The Sampler tab added 2 more
// (sample saved/created): 41 + 2 = 43. The factory Reset added 1
// (sample reset): 43 + 1 = 44. The non-destructive sample-edit descriptor
// added 1 (sample edit changed): 44 + 1 = 45. The event-coverage + undo
// pass added 8 (row volume/pan, insert moved/toggled, send, synth
// committed/reset, audio-panel state): 45 + 8 = 53. The HPF/LPF filter-toggle
// naming gap added 1 (eq filter toggled): 53 + 1 = 54. + lang.changed = 55.
// + undo/redo = 57. pan+zoom promoted out of verbose (+2) = 59.
// + ui.tap (Layer-B interaction telemetry) = 60.
// + popup.opened + popup.closed (portal chokepoint) = 62.
// + ui.scroll (scroll gesture end, coalesced) = 63.
// + ui.search (search field query changed, coalesced) = 64.
// + ui.text (generic text-field commit, Enter/blur) = 65.
// + ui.view (mobile view-mode switch) = 66.
const NumNonVerbose = 66

// IsVerbose reports whether k is one of the high-frequency Verbose*
// kinds that the default sink filters out.
func IsVerbose(k Kind) bool {
	switch k {
	case EventDragProgress, EventInstrumentParamChanged:
		return true
	}
	return false
}

// Source identifies the user-action call site that emitted an Event. It is
// captured at publish time inside the emit helpers (see internal/hooks/source.go
// and internal/ui/event_helpers.go) via runtime.Caller; the goal is for INFO
// log lines and the JSONL eventstream to point at the original user-code
// site, never the middleware (helper / bus / formatter). Zero value means
// "no source captured" — callers using PublishKind directly get this; the
// renderer omits the src= column in that case.
type Source struct {
	Pkg  string `json:"pkg,omitempty"`  // e.g. "internal/ui"
	File string `json:"file,omitempty"` // base file name, e.g. "game_graph_nodes.go"
	Line int    `json:"line,omitempty"`
}

// IsZero reports whether s carries no captured source information.
func (s Source) IsZero() bool { return s.File == "" && s.Pkg == "" && s.Line == 0 }

// String renders the source as "pkg/file.go:line" for log output. Returns
// empty string for the zero value (callers can test cheaply via String()=="").
func (s Source) String() string {
	if s.IsZero() {
		return ""
	}
	if s.Pkg == "" {
		return s.File + ":" + itoa(s.Line)
	}
	return s.Pkg + "/" + s.File + ":" + itoa(s.Line)
}

// itoa is a small allocation-free replacement for strconv.Itoa; keeps the
// hooks package free of strconv (one less stdlib import for a hot path).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// Event is the value delivered to subscribers. Payload is a typed struct
// per Kind (see the *Payload types below). Source identifies the user-code
// site that triggered the publish (zero value when unknown).
type Event struct {
	Kind    Kind
	Payload any
	At      time.Time
	Source  Source
}

// ─── Payload types (one per Kind that carries data) ──────────────────

// NodeEdit is the payload shape for node add/delete/move events.
type NodeEdit struct {
	ID   int    `json:"id"`
	I    int    `json:"i"`
	J    int    `json:"j"`
	Type string `json:"type,omitempty"` // "regular", "silent", "mute", "invisible"
}

// NodeTypePayload describes a node-type change.
type NodeTypePayload struct {
	ID      int    `json:"id"`
	OldType string `json:"old_type"`
	NewType string `json:"new_type"`
}

// NodeParamsPayload describes a node-parameter mutation. Only the
// changed field is non-zero in the typical case.
type NodeParamsPayload struct {
	ID         int     `json:"id"`
	Volume     float64 `json:"volume,omitempty"`
	Pitch      float64 `json:"pitch,omitempty"`
	Duration   float64 `json:"duration,omitempty"`
	LogicKind  string  `json:"logic_kind,omitempty"`
	LogicN     int     `json:"logic_n,omitempty"`
	LogicP     float64 `json:"logic_p,omitempty"`
	GrooveKind string  `json:"groove_kind,omitempty"`
	GroovePct  float64 `json:"groove_pct,omitempty"`
}

// EdgeEdit is the payload for edge add/delete events.
type EdgeEdit struct {
	FromID int `json:"from_id"`
	ToID   int `json:"to_id"`
	FromI  int `json:"from_i"`
	FromJ  int `json:"from_j"`
	ToI    int `json:"to_i"`
	ToJ    int `json:"to_j"`
}

// StartNodePayload describes which node became the start/origin of a row.
type StartNodePayload struct {
	Row int `json:"row"`
	ID  int `json:"id"`
}

// RowChangePayload covers row added/deleted/instrument/mute/solo events.
// Only the relevant fields are populated per Kind. OldInstrument is
// populated only for EventRowInstrumentChange (and stays empty for the
// other row events).
type RowChangePayload struct {
	Row           int     `json:"row"`
	Instrument    string  `json:"instrument,omitempty"`
	OldInstrument string  `json:"old_instrument,omitempty"`
	Name          string  `json:"name,omitempty"`
	Mute          bool    `json:"mute,omitempty"`
	Solo          bool    `json:"solo,omitempty"`
	Volume        float64 `json:"volume,omitempty"`
	Pan           float64 `json:"pan,omitempty"`
}

// SeekPayload describes a transport seek to a particular beat.
type SeekPayload struct {
	Beats int `json:"beats"`
}

// SubdivPayload describes a subdivisions change.
type SubdivPayload struct {
	Subdiv int `json:"subdiv"`
}

// LengthPayload describes a transport length change (in subdivisions).
type LengthPayload struct {
	Length int `json:"length"`
}

// BPMPayload describes a BPM change. Replaces the legacy raw-int payload
// to give the JSONL stream and formatters a consistent typed shape.
type BPMPayload struct {
	BPM int `json:"bpm"`
}

// ImportPayload describes a project-import completion. Bytes is the raw
// payload size; Nodes/Rows are the node and drum-row counts in the imported
// graph (post-validation). The formatter renders all three so the INFO
// line ("import completed (12 nodes, 3 rows, 4521 bytes)") gives the user
// enough context to recognize the imported project.
type ImportPayload struct {
	Bytes int `json:"bytes,omitempty"`
	Nodes int `json:"nodes,omitempty"`
	Rows  int `json:"rows,omitempty"`
}

// MasterVolumePayload describes a master volume slider commit.
type MasterVolumePayload struct {
	Volume float64 `json:"volume"`
}

// EQBandPayload describes an EQ band gain change.
type EQBandPayload struct {
	Channel string  `json:"channel"`
	Band    int     `json:"band"`
	GainDB  float64 `json:"gain_db"`
}

// EQFilterPayload describes a master/channel EQ HPF/LPF enable toggle. Channel
// is the instrument id ("main", "kick", …), Filter is "hpf" or "lpf".
type EQFilterPayload struct {
	Channel  string  `json:"channel"`
	Filter   string  `json:"filter"`
	Enabled  bool    `json:"enabled"`
	CutoffHz float64 `json:"cutoff_hz,omitempty"`
}

// InsertEffectPayload covers insert-effect added/removed/param events.
type InsertEffectPayload struct {
	Channel  string  `json:"channel"`
	Slot     int     `json:"slot"`
	Type     string  `json:"type,omitempty"`
	Param    string  `json:"param,omitempty"`
	Value    float64 `json:"value,omitempty"`
	Enabled  bool    `json:"enabled,omitempty"`
	FromSlot int     `json:"from_slot,omitempty"`
	ToSlot   int     `json:"to_slot,omitempty"`
}

// InstrumentParamPayload describes a per-instrument synth-recipe parameter
// edit. Channel is the instrument id (e.g. "snare"), Recipe is the
// SynthRecipe ID it resolves through (e.g. "drum-snare"), Param is the
// ParamDef.Name, and Value is the new scalar.
type InstrumentParamPayload struct {
	Channel string  `json:"channel"`
	Recipe  string  `json:"recipe,omitempty"`
	Param   string  `json:"param"`
	Value   float64 `json:"value"`
}

// RowColorPayload describes a drum-row color pick.
type RowColorPayload struct {
	Row   int    `json:"row"`
	Color uint32 `json:"color"` // 0xRRGGBBAA
}

// CustomWAVPayload describes a user-loaded WAV becoming available as an
// instrument. IsUpdate is true when the same instrument id was already loaded
// (the new bytes replace the old).
type CustomWAVPayload struct {
	InstrumentID string `json:"instrument_id"`
	IsUpdate     bool   `json:"is_update,omitempty"`
}

// InstrumentRenamePayload describes an instrument id rename.
type InstrumentRenamePayload struct {
	OldID string `json:"old_id"`
	NewID string `json:"new_id"`
}

// ScenePayload describes a scene preset being applied.
type ScenePayload struct {
	Name string `json:"name"`
}

// UIStatePayload describes a UI-state JSON file being applied (camera +
// splitter + sidebar layout, distinct from project import).
type UIStatePayload struct {
	Path string `json:"path"`
}

// FavoritePayload describes an instrument being added to or removed from
// the user's favorites set.
type FavoritePayload struct {
	InstrumentID string `json:"instrument_id"`
	IsFavorite   bool   `json:"is_favorite"`
}

// CameraPanPayload (verbose) describes a camera pan delta.
type CameraPanPayload struct {
	DX float64 `json:"dx"`
	DY float64 `json:"dy"`
}

// CameraZoomPayload (verbose) describes a camera zoom delta.
type CameraZoomPayload struct {
	Factor float64 `json:"factor"`
}

// DragProgressPayload (verbose) describes a drag tick on a node.
type DragProgressPayload struct {
	NodeID int `json:"node_id"`
	I      int `json:"i"`
	J      int `json:"j"`
}

// RecipePayload describes a user-recipe lifecycle event (saved /
// created / deleted). BaseRecipe is empty for a save-in-place; populated
// when the action was a clone of an existing recipe. InstrumentID is
// the row instrument the action originated from, when applicable.
type RecipePayload struct {
	RecipeID     string `json:"recipe_id"`
	BaseRecipe   string `json:"base_recipe,omitempty"`
	InstrumentID string `json:"instrument_id,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
}

// SamplePayload describes a Sampler-tab lifecycle event (saved / created).
// SampleID is the instrument id of the baked sample. SourceID is the
// instrument the sample was captured/saved from (empty for a WAV load).
// DisplayName is the user-entered name on Save As; Frames is the baked
// length so sinks can report sample size without the PCM bytes.
type SamplePayload struct {
	SampleID    string `json:"sample_id"`
	SourceID    string `json:"source_id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Frames      int    `json:"frames,omitempty"`
}

// UndoPayload names the action being undone/redone (the registry label of the
// reverted edit, e.g. "adjust EQ").
type UndoPayload struct {
	Label string `json:"label"`
}

// KitPayload describes a kit-apply event. Members carries the
// role→instrumentID map snapshot the kit just rebound. UI for kits lands
// in a later phase; the payload is reserved now so the eventlogger
// coverage test stays satisfied across the rollout.
type KitPayload struct {
	KitID       string            `json:"kit_id"`
	DisplayName string            `json:"display_name,omitempty"`
	Members     map[string]string `json:"members,omitempty"`
}

// SendPayload describes a delay/reverb send change. Kind is "delay" or "reverb".
type SendPayload struct {
	Channel string  `json:"channel"`
	Kind    string  `json:"kind"`
	Value   float64 `json:"value"`
}

// AudioPanelStatePayload describes an audio-panel preference change (spectrum
// slope, Pre overlay, K-20 view, chain A/B taps). Userpref coverage only.
type AudioPanelStatePayload struct {
	Field string `json:"field"`
}

// LanguagePayload describes a UI-language switch.
type LanguagePayload struct {
	Old string `json:"old"`
	New string `json:"new"`
}

// UITapPayload identifies a tapped button by its resolved label (and icon id
// when the button is icon-only).
type UITapPayload struct {
	Label string `json:"label"`
	Icon  string `json:"icon,omitempty"`
}

// PopupPayload identifies a portal overlay by its stable ID (e.g. "inst-menu",
// "color-wheel", "settings"). Reason is set on close only.
type PopupPayload struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
}

// ScrollPayload names the scrolled surface (e.g. "inst-menu", "row-rack",
// "synth-grid").
type ScrollPayload struct {
	Surface string `json:"surface"`
}

// SearchPayload carries the search surface + the (settled) query string.
type SearchPayload struct {
	Surface string `json:"surface"`
	Query   string `json:"query"`
}

// TextPayload carries the field tag + the committed value.
type TextPayload struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// ViewModePayload names the newly-selected mobile view mode.
type ViewModePayload struct {
	Mode string `json:"mode"`
}
