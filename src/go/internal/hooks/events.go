// Package hooks is a non-blocking event bus for user actions: subscribers run off-thread on a shared async pool, so
// neither the audio thread nor Update ever stalls on a slow handler.
package hooks

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

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
	EventNodeCopied        Kind = "node.copied"
	EventNodeCut           Kind = "node.cut"
	EventNodePasted        Kind = "node.pasted"
	EventNodeParamsChanged Kind = "node.params"
	EventStartNodeChanged  Kind = "node.start"
	EventEdgeAdded         Kind = "edge.added"
	EventEdgeDeleted       Kind = "edge.deleted"

	// Arming or cancelling mutates nothing; the committed operation emits its own domain event.
	EventGridModeArmed    Kind = "mode.armed"
	EventGridModeCanceled Kind = "mode.canceled"

	// Node groups (marquee multi-select). All three are document-scope,
	// undoable, non-verbose.
	EventGroupCreated Kind = "group.created"
	EventGroupChanged Kind = "group.changed"
	EventGroupDeleted Kind = "group.deleted"

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

	// Once per gesture at release, never per frame.
	EventRowVolume           Kind = "row.volume"
	EventInsertEffectMoved   Kind = "audio.insert_moved"
	EventInsertEffectToggled Kind = "audio.insert_toggled"
	// Once at knob release; the per-frame edit stays the verbose EventInstrumentParamChanged.
	EventInstrumentParamsCommitted Kind = "audio.synth_committed"
	EventInstrumentParamsReset     Kind = "audio.synth_reset"
	// Userpref coverage only (NOT undoable): audio-panel state persisted.
	EventAudioPanelStateChanged Kind = "uistate.audio_panel"

	// Published on every frame a synth control moves, like the camera and drag kinds: the INFO narrative filters it
	// (IsVerbose) to keep log throughput bounded, while the verbose JSONL stream records it.
	EventInstrumentParamChanged Kind = "verbose.instrument_param"

	// Round 2 additions: user-narrative events with no prior home.
	EventRowColorChanged   Kind = "row.color"
	EventCustomWAVLoaded   Kind = "audio.wav_loaded"
	EventInstrumentRenamed Kind = "audio.instrument_renamed"
	EventSceneApplied      Kind = "scene.applied"
	EventUIStateApplied    Kind = "uistate.applied"
	EventFavoriteToggled   Kind = "favorite.toggled"

	// A reserved name: the HPF/LPF toggles do not emit it yet, so the registry classifies it with an ExcludedReason.
	EventEQFilterToggled Kind = "audio.eq_filter_toggled"

	// Session scope: the locale persists to userprefs, not the project.
	EventLanguageChanged Kind = "lang.changed"

	// A profile-scope theme edit; session scope, persisted to userprefs.
	EventThemeChanged Kind = "theme.changed"
	// EventCircuitThemeChanged fires on a circuit-scope theme edit. It mutates
	// the exported document and is undoable through the snapshot journal.
	EventCircuitThemeChanged Kind = "theme.circuit.changed"

	// Bracket the readiness gate (rendering the circuit's working set before Play is allowed); Complete says whether the
	// gate opened on its deadline rather than on completion.
	EventWarmupStarted  Kind = "warmup.started"
	EventWarmupComplete Kind = "warmup.complete"

	// EventUndo / EventRedo fire when the user reverts / re-applies one edit.
	// ScopeSession (the meta-action itself is never journaled).
	EventUndo Kind = "undo"
	EventRedo Kind = "redo"

	// EventUITap fires once on a button/pill/toggle/keycap press edge (Layer-B
	// interaction telemetry). The domain effect, if any, is a separate event.
	EventUITap Kind = "ui.tap"

	// From the shared overlay portal for every menu, dialog, picker and slider popup; a close's Reason is "explicit",
	// "top" or "dismiss" (Esc / click outside).
	EventPopupOpened Kind = "popup.opened"
	EventPopupClosed Kind = "popup.closed"

	// At gesture end, coalesced so a flick plus its momentum is one line.
	EventScroll Kind = "ui.scroll"

	// Per keystroke at the source, coalesced to one line with the settled query; Query "" means cleared.
	EventSearchChanged Kind = "ui.search"

	// Domain text commits (rename, BPM) use their own events instead.
	EventTextCommitted Kind = "ui.text"

	// EventViewModeChanged fires when the mobile bottom-nav switches the
	// visible view (pads/eq/wave/spectrum/levels/chain/synth/sampler).
	EventViewModeChanged Kind = "ui.view"

	// Instrument save / clone / delete and kit apply. RecipePayload carries the id, its base for a clone and the
	// originating instrument; EventKitApplied carries KitPayload.
	EventRecipeSaved   Kind = "audio.recipe_saved"
	EventRecipeCreated Kind = "audio.recipe_created"
	EventRecipeDeleted Kind = "audio.recipe_deleted"
	EventKitApplied    Kind = "audio.kit_applied"

	// SampleSaved: a baked sample replaces its source instrument in place; SampleCreated: Save As, a new instrument.
	EventSampleSaved   Kind = "audio.sample_saved"
	EventSampleCreated Kind = "audio.sample_created"
	// An instrument reverted to its shipped state; SourceID is the document it reverted to ("" for a user WAV restored to
	// its original buffer), enough to invert.
	EventSampleReset Kind = "audio.sample_reset"
	// The non-destructive edit descriptor set or cleared. The instrument stays a synth: the edit applies to the fresh
	// render at trigger time.
	EventSampleEditChanged Kind = "audio.sample_edit"

	// Pan fires once per gesture at release with the cumulative delta; zoom per tick, coalesced into one trailing line.
	// Both appear in the default narrative.
	EventCameraPan  Kind = "verbose.pan"
	EventCameraZoom Kind = "verbose.zoom"

	// Many times per frame: delivered only in verbose mode.
	EventDragProgress Kind = "verbose.drag"
)

// Verbose kinds stay at the tail, so a subscriber can split by slicing.
var KindAll = []Kind{
	EventPlayStart, EventPlayStop, EventPaused, EventResumed, EventSeek,
	EventRecordStart, EventRecordStop, EventRecordDropped,
	EventBPMChange, EventSubdivChange, EventLengthChange,
	EventImport, EventExport,
	EventNodeAdded, EventNodeDeleted, EventNodeMoved, EventNodeTypeChanged,
	EventNodeCopied, EventNodeCut, EventNodePasted,
	EventNodeParamsChanged, EventStartNodeChanged,
	EventEdgeAdded, EventEdgeDeleted,
	EventGridModeArmed, EventGridModeCanceled,
	EventGroupCreated, EventGroupChanged, EventGroupDeleted,
	EventRowAdded, EventRowDeleted, EventRowInstrumentChange, EventRowMute, EventRowSolo,
	EventMasterVolumeChange, EventEQBandChange,
	EventInsertEffectAdded, EventInsertEffectRemoved, EventInsertEffectParam,
	EventRowVolume,
	EventInsertEffectMoved, EventInsertEffectToggled,
	EventInstrumentParamsCommitted, EventInstrumentParamsReset,
	EventAudioPanelStateChanged,
	EventRowColorChanged, EventCustomWAVLoaded, EventInstrumentRenamed,
	EventEQFilterToggled,
	EventSceneApplied, EventUIStateApplied, EventFavoriteToggled,
	EventLanguageChanged,
	EventThemeChanged, EventCircuitThemeChanged,
	EventWarmupStarted, EventWarmupComplete,
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

// KindAll[:NumNonVerbose] excludes the high-frequency verbose kinds. TestHooks_NumNonVerboseUpdated re-derives it from
// the registry, so a new Kind fails until this moves.
const NumNonVerbose = 76

// IsVerbose reports whether k is one of the high-frequency Verbose*
// kinds that the default sink filters out.
func IsVerbose(k Kind) bool {
	switch k {
	case EventDragProgress, EventInstrumentParamChanged:
		return true
	}
	return false
}

// Captured by the emit helpers through runtime.Caller, so log lines point at the user-code site, never the middleware.
// Zero (PublishKind) = none; the renderer drops the src= column.
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

// Keeps strconv out of this hot path.
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
	ID   int            `json:"id"`
	I    int            `json:"i"`
	J    int            `json:"j"`
	Type model.NodeType `json:"type"`
}

// NodeTypePayload describes a node-type change.
type NodeTypePayload struct {
	ID      int            `json:"id"`
	OldType model.NodeType `json:"old_type"`
	NewType model.NodeType `json:"new_type"`
}

// NodeParamsPayload describes a node-parameter mutation. Only the
// changed field is non-zero in the typical case.
type NodeParamsPayload struct {
	ID         int              `json:"id"`
	Volume     float64          `json:"volume,omitempty"`
	Pitch      float64          `json:"pitch,omitempty"`
	Duration   float64          `json:"duration,omitempty"`
	LogicKind  model.LogicKind  `json:"logic_kind,omitempty"`
	LogicN     int              `json:"logic_n,omitempty"`
	LogicP     float64          `json:"logic_p,omitempty"`
	GrooveKind model.GrooveKind `json:"groove_kind,omitempty"`
	GroovePct  float64          `json:"groove_pct,omitempty"`
	// Chord names the chord a chord node now sounds ("C#m7/E"); set only by
	// the chord builder's edits.
	Chord string `json:"chord,omitempty"`
}

// GroupPayload describes a node-group mutation.
type GroupPayload struct {
	ID    int    `json:"id"`
	Name  string `json:"name,omitempty"`
	Nodes []int  `json:"nodes,omitempty"`
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

// Only the fields relevant to each Kind are set; OldInstrument only for EventRowInstrumentChange.
type RowChangePayload struct {
	Row           int           `json:"row"`
	Instrument    instrument.ID `json:"instrument,omitempty"`
	OldInstrument instrument.ID `json:"old_instrument,omitempty"`
	Name          string        `json:"name,omitempty"`
	Mute          bool          `json:"mute,omitempty"`
	Solo          bool          `json:"solo,omitempty"`
	Volume        float64       `json:"volume,omitempty"`
}

// SeekPayload describes a transport seek to a particular position.
type SeekPayload struct {
	Beats  int        `json:"beats"` // destination whole-beat (Steps/div)
	Steps  int        `json:"steps"` // destination absolute subdivision index
	Origin SeekOrigin `json:"origin"`
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

// All three render ("12 nodes, 3 rows, 4521 bytes"), enough for the user to recognise the project.
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
	Channel channel.ID `json:"channel"`
	Band    int        `json:"band"`
	GainDB  float64    `json:"gain_db"`
}

// EQFilterPayload describes a master/channel EQ HPF/LPF enable toggle. Filter
// is "hpf" or "lpf".
type EQFilterPayload struct {
	Channel  channel.ID `json:"channel"`
	Filter   EQFilter   `json:"filter"`
	Enabled  bool       `json:"enabled"`
	CutoffHz float64    `json:"cutoff_hz,omitempty"`
}

// InsertEffectPayload covers insert-effect added/removed/param events.
type InsertEffectPayload struct {
	Channel  channel.ID `json:"channel"`
	Slot     int        `json:"slot"`
	Type     string     `json:"type,omitempty"`
	Param    string     `json:"param,omitempty"`
	Value    float64    `json:"value,omitempty"`
	Enabled  bool       `json:"enabled,omitempty"`
	FromSlot int        `json:"from_slot,omitempty"`
	ToSlot   int        `json:"to_slot,omitempty"`
}

// InstrumentParamPayload describes a synth parameter edit: the instrument
// edited, the library instrument its document came from (Origin), the
// ParamDef.Name and the new value.
type InstrumentParamPayload struct {
	Instrument instrument.ID `json:"instrument"`
	Origin     instrument.ID `json:"origin,omitempty"`
	Param      string        `json:"param"`
	Value      float64       `json:"value"`
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
	InstrumentID instrument.ID `json:"instrument_id"`
	IsUpdate     bool          `json:"is_update,omitempty"`
}

// InstrumentRenamePayload describes an instrument id rename.
type InstrumentRenamePayload struct {
	OldID instrument.ID `json:"old_id"`
	NewID instrument.ID `json:"new_id"`
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
	InstrumentID instrument.ID `json:"instrument_id"`
	IsFavorite   bool          `json:"is_favorite"`
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

// BaseRecipe empty = saved in place, set = a clone; InstrumentID is the originating row instrument.
type RecipePayload struct {
	RecipeID     instrument.ID `json:"recipe_id"`
	BaseRecipe   instrument.ID `json:"base_recipe,omitempty"`
	InstrumentID instrument.ID `json:"instrument_id,omitempty"`
	DisplayName  string        `json:"display_name,omitempty"`
}

// SourceID is the instrument it was captured from (empty for a WAV); Frames reports size without the PCM.
type SamplePayload struct {
	SampleID    instrument.ID `json:"sample_id"`
	SourceID    instrument.ID `json:"source_id,omitempty"`
	DisplayName string        `json:"display_name,omitempty"`
	Frames      int           `json:"frames,omitempty"`
}

// UndoPayload names the action being undone/redone (the registry label of the
// reverted edit, e.g. "adjust EQ").
type UndoPayload struct {
	Label string `json:"label"`
}

// The role → instrument map the kit just rebound.
type KitPayload struct {
	KitID       string                            `json:"kit_id"`
	DisplayName string                            `json:"display_name,omitempty"`
	Members     map[instrument.Role]instrument.ID `json:"members,omitempty"` // role → instrument
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

// WarmupPayload describes one playback-readiness warm-up pass.
type WarmupPayload struct {
	Total     int     `json:"total"`    // items that needed rendering
	Rendered  int     `json:"rendered"` // items completed before the gate opened
	Cached    int     `json:"cached"`   // items skipped because they were already warm
	ElapsedMS float64 `json:"elapsed_ms"`
	TimedOut  bool    `json:"timed_out"`
}

// ThemePayload describes a theme edit. Category/Swatch are empty for a
// reset-all; Swatch is empty for a per-category reset. Override is non-nil
// only for the override toggle.
type ThemePayload struct {
	Scope    ThemeScope `json:"scope"`
	Category string     `json:"category,omitempty"`
	Swatch   string     `json:"swatch,omitempty"`
	Override *bool      `json:"override,omitempty"`
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

// GridModePayload describes an armed grid-mode transition (move/copy/connect)
// on the subject node.
type GridModePayload struct {
	Mode   string `json:"mode"`
	ID     int    `json:"id"`
	I      int    `json:"i"`
	J      int    `json:"j"`
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
