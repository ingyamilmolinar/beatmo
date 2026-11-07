// Package timeline keeps per-row drum history with an immutable past: Playback / Import commits never change and edits
// reach only the future; preview.BuildRowWindow merges past, present and future by one precedence table. A bounded
// commit ring per row, with an immutables sidecar keeping playback/import entries the ring trims.
package timeline

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Read-only, and valid only during the callback.
type SegmentsView struct {
	Offset    int
	Past      []bool
	PastMask  []bool
	PastTypes []model.NodeType
	Present   []bool
	Future    []bool
}

// Provenance, so masking tells immutable history from speculation. Playback (real sequencer events) and Import (loaded
// state) are immutable; Seeded (path-change padding) yields to the live predictor on conflict; GapPad is always
// overridable; Released is a seed reconciled to the predictor, bookkeeping that behaves like Playback.
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
	// Reads tier immutables → ring → archive (valueLocked), so scroll-back resolves past immutablesPerRowMax.
	archives map[int]*rowArchive
	// Passes a backward seek abandoned (RewindTo), kept under the full-history rule but NEVER read by valueLocked: answering
	// live lookups from them made an abandoned pass read as history for cells not yet played again.
	superseded map[int]*rowArchive
	// archiveMaxEntries bounds each row's archive size. <= 0 means unbounded.
	archiveMaxEntries int
	// played is the immutable record of what the grid showed for each cell as
	// the playhead passed it (see played.go). Keyed by row.
	played map[int]*playedLog
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
