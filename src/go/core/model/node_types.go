package model

import "github.com/ingyamilmolinar/beatmo/internal/enum"

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
	// NodeTypeChord fires like a regular node but sounds a voice stack
	// derived from Params.Chord (see chord.go / NodeLayers).
	NodeTypeChord
)

var nodeTypeNames = enum.Names[NodeType]{Kind: "node type", Names: []string{
	"regular", "invisible", "silent", "mute", "chord",
}}

// String is the node type's one spelling (project file, event log, JS).
func (t NodeType) String() string                { return enum.String(nodeTypeNames, t) }
func (t NodeType) MarshalText() ([]byte, error)  { return enum.Marshal(nodeTypeNames, t) }
func (t *NodeType) UnmarshalText(b []byte) error { return enum.Unmarshal(nodeTypeNames, b, t) }

// ParseNodeType reads a node type spelled in data.
func ParseNodeType(s string) (NodeType, error) { return enum.Parse(nodeTypeNames, s) }

// LogicKind is a node's built-in trigger rule.
type LogicKind int

const (
	LogicNone            LogicKind = iota // always fire
	LogicEveryN                           // fire on every Nth trigger
	LogicSkipEveryN                       // skip every Nth trigger
	LogicProbability                      // fire with probability LogicP (0..1)
	LogicIfPrevSkipped                    // fire only if the previous regular node skipped
	LogicIfPrevTriggered                  // fire only if the previous regular node fired
)

var logicKindNames = enum.Names[LogicKind]{Kind: "node logic", Names: []string{
	"", "every_n_triggers", "skip_every_n", "probability", "trigger_if_prev_skipped", "trigger_if_prev_triggered",
}}

func (k LogicKind) String() string                { return enum.String(logicKindNames, k) }
func (k LogicKind) MarshalText() ([]byte, error)  { return enum.Marshal(logicKindNames, k) }
func (k *LogicKind) UnmarshalText(b []byte) error { return enum.Unmarshal(logicKindNames, b, k) }

// ParseLogicKind reads a logic rule spelled in data ("" is LogicNone).
func ParseLogicKind(s string) (LogicKind, error) { return enum.Parse(logicKindNames, s) }

// LogicKinds lists every rule, LogicNone first.
func LogicKinds() []LogicKind { return enum.All(logicKindNames) }

// GrooveKind is a node's timing offset direction.
type GrooveKind int

const (
	GrooveNone  GrooveKind = iota // on the grid
	GrooveDelay                   // late by GroovePct of a step
	GrooveRush                    // early by GroovePct of a step
)

var grooveKindNames = enum.Names[GrooveKind]{Kind: "node groove", Names: []string{"", "delay", "rush"}}

func (k GrooveKind) String() string                { return enum.String(grooveKindNames, k) }
func (k GrooveKind) MarshalText() ([]byte, error)  { return enum.Marshal(grooveKindNames, k) }
func (k *GrooveKind) UnmarshalText(b []byte) error { return enum.Unmarshal(grooveKindNames, b, k) }

// ParseGrooveKind reads a groove spelled in data ("" is GrooveNone).
func ParseGrooveKind(s string) (GrooveKind, error) { return enum.Parse(grooveKindNames, s) }

// Seconds is a node's groove nudge: pct (clamped to 0..1) of one step of
// stepSec, late for GrooveDelay, early for GrooveRush, 0 on the grid. The one
// rule the sequencer and the offline renderer both play.
func (k GrooveKind) Seconds(pct, stepSec float64) float64 {
	pct = max(0, min(1, pct))
	switch k {
	case GrooveDelay:
		return pct * stepSec
	case GrooveRush:
		return -pct * stepSec
	}
	return 0
}

// Volume/Pitch/Duration multiply onto row/global settings (defaults 1, 0, 1). Logic may adjust or disable a node per
// trigger and suggest RouteNext; the model stays traversal-agnostic and the UI decides whether to honour it.
type NodeParams struct {
	Volume   float64   // multiplicative gain (default 1)
	Pitch    float64   // semitone offset (default 0)
	Duration float64   // time multiplier (default 1)
	Logic    NodeLogic // optional user logic
	// LogicKind selects a built-in logic rule (LogicNone = always fire).
	LogicKind LogicKind
	LogicN    int
	LogicP    float64
	// Groove: a per-node delay or rush by GroovePct (0..1) of one step.
	GrooveKind GrooveKind
	GroovePct  float64 // 0..1 fraction of one subdivision length

	// Per-node effect overrides (model-only, no UI in V1).
	EffectOverrides []EffectOverride

	// Chord is the chord node's spec (read only while the type is
	// NodeTypeChord, kept on every type — see chord.go).
	Chord ChordSpec
}

// EffectOverride modulates a single parameter on an insert effect for a specific node.
type EffectOverride struct {
	Type  string  `json:"type"`  // effect type name (e.g. "distortion")
	Param string  `json:"param"` // parameter name (e.g. "drive")
	Value float64 `json:"value"` // override value
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
