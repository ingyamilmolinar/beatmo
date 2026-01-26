package model

const InvalidNodeID NodeID = -1

type NodeID int

type Node struct {
	I, J int
	Type NodeType // Node type (audible/silent/invisible)
	// Params define per-node playback parameters and optional user logic.
	Params NodeParams
}

// NodeType defines the type of a node.
type NodeType int

const (
	// NodeTypeRegular is a visible, audible node.
	NodeTypeRegular NodeType = iota
	// NodeTypeInvisible is not drawn and never plays audio.
	NodeTypeInvisible
	// NodeTypeSilent is visible but never plays audio. Useful for
	// orthogonal routing without introducing audible triggers at corners.
	NodeTypeSilent
	// NodeTypeMute is visible and treated as a timeline event, but it actively
	// mutes the instrument so no sound is produced while it is in effect.
	NodeTypeMute
)

// NodeParams centralizes per‑node behavior and is kept within the model so
// callers (UI, engine) can query a node’s intent without scattering logic.
//
// Volume/Pitch/Duration are multiplicative adjustments applied on top of any
// row/global settings. They default to 1, 0, 1 respectively.
//
// Logic, when present, can further adjust playback parameters or disable a
// node on a per‑trigger basis. It may also suggest routing decisions via
// RouteNext, which higher layers may optionally use. The model itself remains
// traversal‑agnostic; UI decides whether to honor routing suggestions.
type NodeParams struct {
	Volume   float64   // multiplicative gain (default 1)
	Pitch    float64   // semitone offset (default 0)
	Duration float64   // time multiplier (default 1)
	Logic    NodeLogic // optional user logic
	// SkipEveryN is deprecated (back-compat only). SetNodeParams normalizes it
	// into LogicKind="skip_every_n" + LogicN and then clears it.
	SkipEveryN int
	// LogicKind selects a built-in logic rule. Empty means none. Supported:
	//  "every_n_triggers"  – fire on every Nth trigger (complement of skip)
	//  "skip_every_n"      – skip on every Nth trigger
	//  "probability"       – fire with probability P (0..1)
	//  "trigger_if_prev_skipped"   – fire only if previous regular skipped
	//  "trigger_if_prev_triggered" – fire only if previous regular triggered
	LogicKind string
	LogicN    int
	LogicP    float64
	// Groove parameters (per-node): one of none|delay|rush with percentage 0..1
	GrooveKind string  // ""|"delay"|"rush"
	GroovePct  float64 // 0..1 fraction of one subdivision length
}

// NodeLogic computes per‑trigger behavior for a node.
type NodeLogic func(NodeContext) NodeDecision

// NodeContext describes the current trigger in a timeline.
type NodeContext struct {
	NodeID        NodeID
	Row           int // drum row index (if applicable)
	AbsoluteIndex int // absolute subdivision index in the timeline
	TriggerCount  int // 1‑based count of times this node has been triggered for the row
}

// NodeDecision returned by NodeLogic. Zero values imply no change.
type NodeDecision struct {
	Enabled     *bool    // if set and false, suppress playback for this trigger
	VolumeMul   float64  // multiplicative gain (default 1 when 0)
	PitchDelta  float64  // additional semitones (default 0)
	DurationMul float64  // multiplicative time (default 1 when 0)
	RouteNext   []NodeID // optional suggested next outputs
}

// BeatInfo holds information about a beat in the drum row.
type BeatInfo struct {
	NodeID   NodeID
	NodeType NodeType
	I, J     int // Grid coordinates for this beat
}
