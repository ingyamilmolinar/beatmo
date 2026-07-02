package ui

import (
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
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
	// GenAtRecord is the parity generation at which the contributing buffered
	// event (audio event / seq decision / highlight) was recorded.
	// GenAtScan is the generation at which the scan ran. They will agree on a
	// real mismatch; divergence means the scan saw cross-generation state and
	// the entry was already gen-filtered out.
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
	Inst  string
	Vol   float64
	Pitch float64
	Dur   float64
	// Gen is the audio scheduler's replay generation (g.audioGen) — bumped on
	// stop/replay to drop in-flight notes from the previous run.
	Gen uint64
	// ParityGen is the structural-mutation generation (g.parityGen) at the
	// moment this event was recorded. parityScan ignores events whose
	// ParityGen disagrees with the current generation — that protects against
	// runtime mutations (instrument change, EQ edit, BPM, etc.) racing the
	// audio thread.
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
	// Enqueued reports whether the scheduler actually handed this beat's audio
	// to the audio pipeline (queued into audioCh). In production every audible,
	// non-gated regular decision enqueues its audio in the SAME seqMu critical
	// section that records the decision, so Audible⟹Enqueued is an invariant.
	// The audio_missing parity check uses this to distinguish a genuine
	// scheduler bug (decided audible but never enqueued — Enqueued=false) from
	// audio that is merely still in-flight in the channel or was dropped by the
	// audio thread under backpressure/transport transitions (Enqueued=true).
	// Without it, a beat whose audio is queued but not yet dispatched by the
	// audioLoop trips a false-positive audio_missing once the 120ms grace
	// (anchored to the decision timestamp) expires.
	Enqueued bool
	// ParityGen is the structural-mutation generation at the moment this seq
	// decision was recorded. The scan filters by current ParityGen so prior-
	// generation decisions never participate in comparisons.
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
