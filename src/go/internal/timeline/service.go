package timeline

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// NewService constructs an empty timeline service.
func NewService() *Service {
	return &Service{
		immutables:        make(map[int]map[int]commitEntry),
		archives:          make(map[int]*rowArchive),
		archiveMaxEntries: archiveDefaultMaxEntries,
	}
}

// SetArchiveMaxEntries overrides the per-row cold-archive ceiling. Values <= 0
// disable the ceiling (archive grows without bound, intended only for tests
// that drive abs to known finite limits). Idempotent.
func (s *Service) SetArchiveMaxEntries(n int) {
	s.mu.Lock()
	s.archiveMaxEntries = n
	s.mu.Unlock()
}

// ArchiveLenForTest reports the cold-archive entry count for the row.
func (s *Service) ArchiveLenForTest(row int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if a, ok := s.archives[row]; ok {
		return a.lenForTest()
	}
	return 0
}

// ensureArchiveLocked returns (creating if necessary) the per-row cold archive.
// Caller must hold s.mu.
func (s *Service) ensureArchiveLocked(row int) *rowArchive {
	if s.archives == nil {
		s.archives = make(map[int]*rowArchive)
	}
	a := s.archives[row]
	if a == nil {
		a = &rowArchive{}
		s.archives[row] = a
	}
	return a
}

// commitRingCapMultiplier scales the caller's capacity hint to determine the
// hard ceiling on the per-row commitRing. Eight loop-lengths gives enough
// past-history depth for reconcileFrozen on the visible window while keeping
// long-session memory bounded — without this, the ring doubled forever and
// participated in the production WASM OOM (see commit_ring_bound_test.go).
const commitRingCapMultiplier = 8

// immutablesPerRowMax bounds the immutables sidecar map per row. Mirrors
// parityAudioMax = 1024 (game_parity_state.go:150). Older entries are dropped
// at write time; reconcileFrozen only queries abs values inside the visible
// window (game_refresh_drum_row.go:115), so dropped history is not consulted.
const immutablesPerRowMax = 1024

// RingLenForTest exposes the per-row commit ring size and capacity plus the
// immutables sidecar size. Test-only diagnostic for soak/regression tests
// that assert bounded growth: the underlying ring and the immutables map are
// unexported, so this is the canonical seam.
func (s *Service) RingLenForTest(row int) (ringSize, ringCap, immutables int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if row >= 0 && row < len(s.rows) {
		ringSize = s.rows[row].commits.size
		ringCap = len(s.rows[row].commits.vals)
	}
	if m, ok := s.immutables[row]; ok {
		immutables = len(m)
	}
	return
}

// RecordCommit stores a past timeline entry for a row. Capacity hints control
// how many historical entries are retained.
func (s *Service) RecordCommit(row, abs int, val bool, typ model.NodeType, windowLen, capacity int) {
	s.RecordCommitKind(row, abs, val, typ, CommitKindPlayback, windowLen, capacity)
}

// RecordCommitKind is the provenance-aware version of RecordCommit.
func (s *Service) RecordCommitKind(row, abs int, val bool, typ model.NodeType, kind CommitKind, windowLen, capacity int) {
	if row < 0 {
		return
	}
	if windowLen < 0 {
		windowLen = 0
	}
	if capacity < windowLen {
		capacity = windowLen
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seg := s.ensureRow(row, windowLen)
	if seg == nil {
		return
	}
	// Lock in the per-row ring ceiling on first commit. setCapMax is idempotent;
	// the largest capacity ever requested defines the bound.
	seg.commits.setCapMax(capacity * commitRingCapMultiplier)
	// If the immutable sidecar already contains a playback/import entry for this
	// absolute index, never overwrite it—even if the ring has trimmed the entry.
	// This is critical for ensuring past immutability during long sessions where
	// the ring buffer no longer holds the earliest commits.
	if m := s.immutables[row]; m != nil {
		if _, ok := m[abs]; ok {
			return
		}
	}
	if _, _, k, ok := seg.commits.value(abs); ok {
		if k == CommitKindPlayback || k == CommitKindImport {
			// Preserve immutable history; ignore overwrite attempts.
			return
		}
		// For mutable kinds, allow replacement in-place.
		seg.commits.replace(abs, val, typ, kind)
		if kind == CommitKindPlayback || kind == CommitKindImport {
			if s.immutables == nil {
				s.immutables = make(map[int]map[int]commitEntry)
			}
			rowMap := s.immutables[row]
			if rowMap == nil {
				rowMap = make(map[int]commitEntry)
				s.immutables[row] = rowMap
			}
			rowMap[abs] = commitEntry{val: val, typ: typ, kind: kind}
		}
		return
	}
	seg.commits.ensureCapacity(capacity)
	seg.commits.append(abs, val, typ, kind)
	if kind == CommitKindPlayback || kind == CommitKindImport {
		if s.immutables == nil {
			s.immutables = make(map[int]map[int]commitEntry)
		}
		rowMap := s.immutables[row]
		if rowMap == nil {
			rowMap = make(map[int]commitEntry)
			s.immutables[row] = rowMap
		}
		rowMap[abs] = commitEntry{val: val, typ: typ, kind: kind}
		s.pruneImmutablesRowLocked(row, abs)
	}
}

// pruneImmutablesRowLocked migrates entries below the retention window from
// the live sidecar into the cold archive so the per-row map stays within
// immutablesPerRowMax. Caller must hold s.mu. Playback/import entries are
// preserved (in the archive, not deleted); other kinds are dropped because
// the user-visible scroll-back contract only covers true playback history.
func (s *Service) pruneImmutablesRowLocked(row, latestAbs int) {
	rowMap := s.immutables[row]
	if len(rowMap) <= immutablesPerRowMax {
		return
	}
	minKeep := latestAbs - immutablesPerRowMax + 1
	var arch *rowArchive
	for k, ce := range rowMap {
		if k >= minKeep {
			continue
		}
		if ce.kind == CommitKindPlayback || ce.kind == CommitKindImport {
			if arch == nil {
				arch = s.ensureArchiveLocked(row)
			}
			arch.append(k, ce.val, ce.typ, ce.kind, s.archiveMaxEntries)
		}
		delete(rowMap, k)
	}
}
