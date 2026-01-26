package timeline

import (
	"sync"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// SegmentsView exposes live timeline slices while the caller-provided callback
// is executing. The slices must be treated as read-only and are only valid
// during the callback.
type SegmentsView struct {
	Offset    int
	Past      []bool
	PastMask  []bool
	PastTypes []model.NodeType
	Present   []bool
	Future    []bool
}

// CommitKind tracks the provenance of a timeline commit so masking policy can
// distinguish immutable playback history from speculative seeds or gap pads.
//
// Kind semantics:
//   - Playback: written from real sequencer/audio events; must remain immutable.
//   - Import:   loaded from persisted session state; treated as immutable.
//   - Seeded:   opportunistic history seeds (e.g., path-change padding); may be
//     replaced by live predictor if they conflict.
//   - GapPad:   automatically padded invisible gaps between commits; always
//     overridable.
//   - Released: a previously seeded entry that was reconciled to predictor; it
//     serves as bookkeeping for debugging but behaves like Playback.
type CommitKind int

const (
	CommitKindPlayback CommitKind = iota
	CommitKindImport
	CommitKindSeeded
	CommitKindGapPad
	CommitKindReleased
)

// Snapshot is an immutable copy of the segments for external consumers (tests,
// JS bridge).
type Snapshot struct {
	Offset    int
	Past      []bool
	PastMask  []bool
	PastTypes []model.NodeType
	Present   []bool
	Future    []bool
}

// Service coordinates per-row drum timeline history, keeping committed past
// entries while exposing present/future slices derived from the latest graph
// state.
type Service struct {
	mu   sync.RWMutex
	rows []rowState
	// immutables keeps a copy of playback/import commits keyed by row+abs so
	// they remain available even if the ring buffer is trimmed or resized.
	immutables map[int]map[int]commitEntry
}

type rowState struct {
	offset    int
	windowLen int

	past      []bool
	pastTypes []model.NodeType
	pastMask  []bool
	present   []bool
	future    []bool

	commits commitRing
}

type commitEntry struct {
	val  bool
	typ  model.NodeType
	kind CommitKind
}

// PathChangeTrimReport describes how TrimAfterPathChange reconciled the
// timeline after a beat-path mutation.
type PathChangeTrimReport struct {
	MaxKeep       int
	LastImmutable int
}
