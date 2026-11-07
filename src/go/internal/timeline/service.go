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

// <= 0 disables the ceiling (tests driving abs to known finite limits only).
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

// ensureSupersededLocked returns (creating if necessary) the per-row store of
// passes abandoned by a backward seek. Caller must hold s.mu.
func (s *Service) ensureSupersededLocked(row int) *rowArchive {
	if s.superseded == nil {
		s.superseded = make(map[int]*rowArchive)
	}
	a := s.superseded[row]
	if a == nil {
		a = &rowArchive{}
		s.superseded[row] = a
	}
	return a
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

// Eight loop lengths give reconcileFrozen its lookback on the visible window while keeping long sessions bounded.
const commitRingCapMultiplier = 8

// reconcileFrozen queries only the visible window, so history dropped from here is never consulted.
const immutablesPerRowMax = 1024

// The ring and the map are unexported; this is the seam soak tests assert bounded growth through.
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
	// Set on first commit; the largest capacity ever requested is the bound.
	seg.commits.setCapMax(capacity * commitRingCapMultiplier)
	// A playback/import entry in the sidecar is never overwritten, even after the ring trimmed it: past immutability in
	// long sessions depends on it.
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

// Below the retention window Playback/Import migrate to the archive (kept), other kinds drop: scroll-back covers only
// real playback. Caller holds s.mu.
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
