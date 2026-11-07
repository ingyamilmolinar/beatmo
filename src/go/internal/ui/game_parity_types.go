package ui

import (
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// mismatchEntry captures a single scheduler-vs-UI discrepancy for diagnostics.
type mismatchEntry struct {
	Row       int
	Abs       int
	Kind      string
	NodeType  model.NodeType
	Scheduled bool // scheduler decided to play/trigger
	Slate     bool // DrumView slate value at the same abs
	Source    string
	Offset    int
	Length    int
	InWindow  bool
	RowMuted  bool
	AnySolo   bool
	Missing   bool
	Expected  bool
	Actual    bool
	When      float64
	Force     bool
	Detail    string
	// They agree on a real mismatch; divergence means the scan saw cross-generation state.
	GenAtRecord uint64
	GenAtScan   uint64
}

// mismatchRing is a fixed-capacity ring buffer for recent mismatches.
type mismatchRing struct {
	mu      sync.Mutex
	entries []mismatchEntry
	head    int
	size    int
	cap     int
}

func newMismatchRing(capacity int) mismatchRing {
	if capacity <= 0 {
		capacity = 64
	}
	return mismatchRing{entries: make([]mismatchEntry, capacity), cap: capacity}
}

func (r *mismatchRing) add(e mismatchEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cap == 0 {
		return
	}
	r.entries[r.head] = e
	r.head = (r.head + 1) % r.cap
	if r.size < r.cap {
		r.size++
	}
}

func (r *mismatchRing) snapshot() []mismatchEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]mismatchEntry, r.size)
	if r.size == 0 {
		return out
	}
	start := (r.head - r.size + r.cap) % r.cap
	for i := 0; i < r.size; i++ {
		out[i] = r.entries[(start+i)%r.cap]
	}
	return out
}

func (r *mismatchRing) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.head = 0
	r.size = 0
}

type parityAudioEvent struct {
	Row   int
	Abs   int
	When  float64
	Inst  instrument.ID
	Vol   float64
	Pitch float64
	Dur   float64
	// Bumped on stop/replay to drop the previous run's in-flight notes.
	Gen uint64
	// parityScan ignores events from an older generation, so mutations cannot race the audio thread.
	ParityGen  uint64
	RecordedAt time.Time
}

type paritySeqDecision struct {
	Row      int
	Abs      int
	Audible  bool // audio truth (AudibleAt)
	Visible  bool // view truth (VisibleAt)
	NodeType model.NodeType
	Missing  bool
	// Record and enqueue share one seqMu section, so Audible ⟹ Enqueued. audio_missing uses this to
	// tell a real scheduler bug from audio still in flight or dropped downstream.
	Enqueued bool
	// Older-generation decisions never take part in comparisons.
	ParityGen  uint64
	RecordedAt time.Time
}

func boolSliceDebug(src []bool) []int {
	out := make([]int, len(src))
	for i, v := range src {
		if v {
			out[i] = 1
		}
	}
	return out
}
