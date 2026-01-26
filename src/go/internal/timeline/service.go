package timeline

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
)


// NewService constructs an empty timeline service.
func NewService() *Service {
	return &Service{immutables: make(map[int]map[int]commitEntry)}
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
	}
}

