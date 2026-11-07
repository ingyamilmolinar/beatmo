package ui

import (
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// Row-indexed scheduling, beat-path, freeze and trigger state shared by Update, the sequencer and the row
// refresh; value-embedded in Game so its fields stay promoted. Holds atomics: never a composite literal or a copy.
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
	// Ratchets within one transport generation so UI timings can grow the cushion but never make it oscillate.
	// Under seqMu.
	lookaheadFloor float64
	lookaheadGen   uint64
	// Immutable, so path rebuilds never race with scheduling.
	seqPathSnap atomic.Pointer[seqPathSnapshot]

	// Stops a cell refreshed across frames from firing its highlight hook twice.
	lastHLIdxByRow []int

	// highlight scheduling from sequencer goroutine
	hlCh chan struct {
		row, idx int
		info     model.BeatInfo
	}

	// Segmented drum timeline; stores committed past beats per row.
	timeline *timeline.Service
	// Inclusive; -1 at start, monotonic while playing, reset on Stop.
	frozenUpToByRow []int

	// Lets tests assert the back-fill is bounded by the predictor window, not the elapsed playhead: a full re-freeze
	// on resume hung playback after a long session.
	freezeBackfillIters int64

	// The scan resumes from here, bounding it to the newly frozen cells; -1 = not yet. Invalidated on SetPaths,
	// Stop, a row-count change and a clamp/trim.
	reconciledUpToByRow []int

	// So stable rows are not recomputed during playback.
	pathSigByRow        []uint64
	rowsPathChanged     []bool
	pathChangeBeatByRow []int
	pathsDirty          bool

	// So an edit during playback invalidates only the rows that changed.
	rowRenderSig []uint64
	// So published Steps/CellTypes are never mutated in place; rowSnapshotMode allocates fresh slices instead.
	rowStepsScratch [][]bool
	rowTypesScratch [][]model.NodeType
	rowSnapshotMode bool
	// A stationary window keeps its past cells; a rewound one recomputes from the predictor.
	lastStepsOffset     int
	lastCellTypesOffset int

	// The GLOBAL transport generation: Pause/Stop/seek bump it to drop all in-flight audio. A structural edit
	// must NOT (it dropped scheduled beats on unchanged rows); that is per-row via Game.rowStructGen.
	audioGen atomic.Uint64

	// Per-row trigger counters for node-level logic evaluation.
	nodeTriggerCountsByRow map[int]map[model.NodeID]int
	// Guards against counting the same (row, idx, node) twice in a frame.
	lastEvalIdxByRowNode map[int]map[model.NodeID]int
	// Per-row trigger counters for NodeLogic callbacks (separate from built-in logic).
	nodeLogicTriggerCountsByRow map[int]map[model.NodeID]int

	// For logic rules that depend on whether a node fired last time.
	lastTriggeredByRow map[int]map[model.NodeID]bool

	// Last audible node fired on each row; used by built-in logic rules.
	lastFiredNodeByRow []model.NodeID
	// Exclusive.
	muteUntilByRow []int
	// Loop completion counts per row.
	loopCountByRow []int
	// Keyed by origin node, so undoing a delete brings the row's past back.
	playedGraveyard map[model.NodeID]*timeline.PlayedHistory
}
