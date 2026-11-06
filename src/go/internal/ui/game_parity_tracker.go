package ui

import (
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// parityTracker groups the parity-diagnostics state that cross-checks the audio
// scheduler against the DrumView slate (see game_parity_*.go). It is embedded
// anonymously in Game so existing g.parityRing / g.parityGen / g.parityScan* …
// call sites keep resolving via field promotion; extracting it just lifts these
// ~20 diagnostic fields out of the ~490-field Game struct into one cohesive unit.
//
// The parity methods themselves stay on *Game: they read Game's beat/timeline/
// predictor state (beatInfoAtRow, rowPastExclusive, timelineCommittedWithKind, …)
// so moving them here would only trade promotion for a Game back-pointer.
//
// Concurrency: parityMu guards parityAudio / paritySeqDecisions (written by the
// sequencer/audio side, read by the comparator). parityGen and parityGraceUntilNS
// are the cross-thread coordination spine and MUST stay atomics — they are read
// from the audio thread, the scheduler, and the comparator without holding a lock.
// This struct is embedded by value in *Game (never copied), so the lock/atomics
// stay in place; parityTracker itself must never be copied.
type parityTracker struct {
	parityRing   mismatchRing
	parityStreak map[string]int
	parityWatch  parityWatchMode
	// parityTimingWatch is the per-Game timing-gate watch mode, seeded from
	// parityTimingWatchDefault at construction (mirrors parityWatch).
	parityTimingWatch   parityWatchMode
	parityMu            sync.Mutex
	parityAudio         []parityAudioEvent
	parityAudioMaxIdx   []int
	paritySeqDecisions  map[int]map[int]paritySeqDecision
	parityScanEvery     int
	parityScanStride    int
	parityScanPhase     int
	parityScanLastFrame int64
	parityScanSumNS     int64
	parityScanMaxNS     int64
	parityScanCount     int64
	// Test-only diagnostic counters for parityPrune invocations.
	parityPruneCallsForTest     int64
	parityPruneMaxMinAbsForTest int
	parityScanCallsForTest      int64
	parityScanReturnsForTest    [10]int64 // by early-return slot
	// parityScanStoppedForTest opts ONE test Game into scanning a stopped
	// transport, which production never does. Default false, so tests scan
	// exactly when production does.
	parityScanStoppedForTest bool
	// parityGen monotonically advances on every runtime structural mutation
	// (instrument change, BPM, length, graph edit, row add/del, etc). All
	// parity event records (audio, seq decisions, highlights) carry the gen at
	// which they were recorded; parityScan discards entries whose gen disagrees
	// with the current generation. This is the single coordination spine
	// between the audio thread, the scheduler, and the parity comparator.
	parityGen atomic.Uint64
	// parityGraceUntilNS is a wall-clock deadline (UnixNano). Until this time,
	// parityScan/parityCheck downgrade mismatches to log-only — gives parity
	// buffers and the predictor/timeline state a window to reach coherence
	// after a structural mutation. Stored as int64 (nanoseconds) under
	// atomic.Int64 so concurrent writers from the audio thread don't race.
	parityGraceUntilNS atomic.Int64
	// timingTraces maps (row<<32|abs) → open timing trace, closed when the
	// mixer/WebAudio reports the voice's actual emission. Bounded by pruning
	// alongside the other parity buffers (see openTimingTrace).
	timingTraces map[int64]timingTrace
	// emissionReportBuf is a reusable scratch buffer for draining the mixer's
	// lock-free emission report ring (see drainEmissionReports).
	emissionReportBuf []audio.EmitReport
}
