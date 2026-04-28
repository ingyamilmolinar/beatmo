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

	// Verbose / opt-in (only delivered when verbose mode is enabled on the
	// bus or the eventstream sink). These fire many times per frame; the
	// default sink filters them out.
	EventCameraPan    Kind = "verbose.pan"
	EventCameraZoom   Kind = "verbose.zoom"
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
	EventCameraPan, EventCameraZoom, EventDragProgress,
}

// numNonVerbose is the count of non-verbose kinds in KindAll.
// KindAll[:NumNonVerbose] excludes the high-frequency verbose events.
const NumNonVerbose = 30

// IsVerbose reports whether k is one of the high-frequency Verbose*
// kinds that the default sink filters out.
func IsVerbose(k Kind) bool {
	switch k {
	case EventCameraPan, EventCameraZoom, EventDragProgress:
		return true
	}
	return false
}

// Event is the value delivered to subscribers. Payload is a typed struct
// per Kind (see the *Payload types below).
type Event struct {
	Kind    Kind
	Payload any
	At      time.Time
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
	ID        int     `json:"id"`
	Volume    float64 `json:"volume,omitempty"`
	Pitch     float64 `json:"pitch,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	LogicKind string  `json:"logic_kind,omitempty"`
	LogicN    int     `json:"logic_n,omitempty"`
	LogicP    float64 `json:"logic_p,omitempty"`
	GrooveKind string `json:"groove_kind,omitempty"`
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
	Row           int    `json:"row"`
	Instrument    string `json:"instrument,omitempty"`
	OldInstrument string `json:"old_instrument,omitempty"`
	Name          string `json:"name,omitempty"`
	Mute          bool   `json:"mute,omitempty"`
	Solo          bool   `json:"solo,omitempty"`
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

// InsertEffectPayload covers insert-effect added/removed/param events.
type InsertEffectPayload struct {
	Channel string  `json:"channel"`
	Slot    int     `json:"slot"`
	Type    string  `json:"type,omitempty"`
	Param   string  `json:"param,omitempty"`
	Value   float64 `json:"value,omitempty"`
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
