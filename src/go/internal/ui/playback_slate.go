package ui

import (
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// playbackSlate owns the row-indexed scheduling, beat-path, timeline/freeze,
// and trigger-logic state that Game.Update, the background sequencer, and the
// drum-row refresh share. Value-embedded in Game (see game_struct.go) so every
// field stays promoted (g.nextBeatIdxs, …) — zero call-site churn. Methods are
// relocated onto *playbackSlate in Tasks 2–4; the host interface arrives in Task 5.
//
// Contains atomics (seqPathSnap, audioGen) → NEVER construct a playbackSlate{}
// composite literal and never copy it (copylocks). Value-embed + statement-init.
type playbackSlate struct {
	g *Game // back-reference, set once in New (replaced by a SlateHost interface in Task 5)

	beatInfos          []model.BeatInfo   // Full traversal path for primary start
	drumBeatInfos      []model.BeatInfo   // BeatInfos sized to drum view
	beatInfosByRow     [][]model.BeatInfo // Per-row traversal paths
	isLoop             bool               // Indicates if the current graph forms a loop
	loopStartIndex     int                // The index in beatInfos where the loop segment begins
	isLoopByRow        []bool
	loopStartByRow     []int
	loopLenByRow       []int
	originIdxsByRow    [][]int
	nextOriginIdxByRow []int
	nextBeatIdxs       []int  // Absolute beat index per row
	nextIdxSticky      []bool // Preserve nextBeatIdxs on row shifts
	startNextBuf       []int  // Reusable buffer for frame-start snapshot of nextBeatIdxs
	elapsedBeats       int

	// sequencer (engine-driven audio independent from UI)
	seqCh       <-chan engine.Event
	seqNextIdxs []int
	seqQuit     chan struct{}
	// lookaheadFloor/lookaheadGen ratchet runtimeAudioLookahead within one
	// transport generation (g.audioGen) so UI timings can grow the audio
	// cushion but never make it oscillate. Accessed under seqMu (scheduleSound).
	lookaheadFloor float64
	lookaheadGen   uint64
	// seqPathSnap holds an immutable snapshot of per-row beat paths for the
	// background sequencer so path rebuilds do not race with scheduling.
	seqPathSnap atomic.Pointer[seqPathSnapshot]

	// lastHLIdxByRow tracks the last highlighted absolute subdivision index
	// per row to prevent duplicate highlight hooks when the same cell is
	// refreshed across frames.
	lastHLIdxByRow []int

	// highlight scheduling from sequencer goroutine
	hlCh chan struct {
		row, idx int
		info     model.BeatInfo
	}

	// Segmented drum timeline; stores committed past beats per row.
	timeline *timeline.Service
	// Highest absolute subdivision index frozen per row (inclusive). Starts
	// at -1; grows monotonically while playing. Reset on Stop.
	frozenUpToByRow []int

	// Instrumentation: total iterations executed by refreshDrumRow's safety-net
	// freeze back-fill loop, summed across rows over the process lifetime. Used
	// by tests to assert the loop is bounded by the predictor window rather than
	// the (unbounded) elapsed playhead — a full re-freeze from abs 0 on resume
	// was the "playing after a long session hangs for a few seconds" bug.
	freezeBackfillIters int64

	// Highest absolute subdivision index already validated by reconcileFrozen
	// per row (inclusive). reconcileFrozen's per-refresh scan resumes from
	// reconciledUpToByRow[r]+1 instead of futureReleaseStart, bounding scan
	// cost to O(newly frozen abs since last refresh) instead of O(freezeLimit).
	// -1 means "not yet reconciled". Invalidated on SetPaths, Stop reset, row
	// count change, and inside reconcileFrozen on clamp/trim.
	reconciledUpToByRow []int

	// Path signatures per row to detect circuit shape changes. Used to avoid
	// recomputing stable rows during active playback.
	pathSigByRow        []uint64
	rowsPathChanged     []bool
	pathChangeBeatByRow []int
	pathsDirty          bool

	// Per-row signature of the current rendered row window (Steps + CellTypes)
	// used to invalidate only changed row caches when circuits are edited
	// during active playback.
	rowRenderSig []uint64
	// Scratch buffers used by refreshDrumRow so published Steps/CellTypes are not
	// mutated in place. When rowSnapshotMode is enabled (debug), refreshDrumRow
	// allocates fresh slices instead of reusing these buffers.
	rowStepsScratch [][]bool
	rowTypesScratch [][]model.NodeType
	rowSnapshotMode bool
	// Offsets of the last DrumView window used to populate step/cell metadata.
	// Helps preserve past cells across refreshes when the visible window stays
	// stationary; rewound windows must recompute from predictor snapshots.
	lastStepsOffset     int
	lastCellTypesOffset int

	// audioGen is the GLOBAL transport generation. Pause/Stop/seek bump it to
	// drop ALL in-flight scheduled audio. A structural graph edit must NOT bump
	// it — doing so dropped already-scheduled beats on rows whose path did not
	// change, leaving an audible gap when placing/moving a node. Structural
	// invalidation is per-row via Game.rowStructGen (kept on Game, not this slate).
	audioGen atomic.Uint64

	// Per-row trigger counters for node-level logic evaluation.
	nodeTriggerCountsByRow map[int]map[model.NodeID]int
	// Guard to avoid double-incrementing trigger counters when the same
	// (row, idx, node) is evaluated multiple times within a frame.
	lastEvalIdxByRowNode map[int]map[model.NodeID]int
	// Per-row trigger counters for NodeLogic callbacks (separate from built-in logic).
	nodeLogicTriggerCountsByRow map[int]map[model.NodeID]int

	// Last trigger state for each node per row: true if the last evaluation
	// for that node resulted in an audible trigger. Used by advanced logic
	// rules that depend on previous node trigger/skip.
	lastTriggeredByRow map[int]map[model.NodeID]bool

	// Last audible node fired on each row; used by built-in logic rules.
	lastFiredNodeByRow []model.NodeID
	// Per-row mute gate (absolute subdivision index, exclusive) set by
	// mute nodes to temporarily silence playback.
	muteUntilByRow []int
	// Loop completion counts per row.
	loopCountByRow []int
	// playedGraveyard keeps the played history of rows deleted during this
	// playback session, keyed by the row's origin node, so undoing the delete
	// brings the row's past back with it. Cleared with the timeline.
	playedGraveyard map[model.NodeID]*timeline.PlayedHistory
}
