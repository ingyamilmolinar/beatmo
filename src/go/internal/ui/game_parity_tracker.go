package ui

import (
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Embedded in Game (fields resolve by promotion); the methods stay on *Game because they read its
// state. parityGen and parityGraceUntilNS MUST stay atomics: read across threads without a lock. Never copy.
type parityTracker struct {
	parityRing          mismatchRing
	parityStreak        map[string]int
	parityWatch         parityWatchMode
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
	// Production never scans a stopped transport; this opts ONE test Game in.
	parityScanStoppedForTest bool
	// The single coordination spine between the audio thread, scheduler and comparator: advances on
	// every structural mutation, and every parity record carries the value it was recorded under.
	parityGen atomic.Uint64
	// UnixNano deadline; atomic because the audio thread writes it.
	parityGraceUntilNS atomic.Int64
	// Pruned alongside the other parity buffers.
	timingTraces      map[int64]timingTrace
	emissionReportBuf []audio.EmitReport
}
