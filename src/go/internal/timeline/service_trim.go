package timeline

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TrimBefore drops committed entries older than minAbs.
func (s *Service) TrimBefore(row, minAbs int) {
	if row < 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if row >= len(s.rows) {
		return
	}
	s.rows[row].commits.trimBefore(minAbs)
	if m := s.immutables[row]; m != nil {
		for k := range m {
			if k < minAbs {
				delete(m, k)
			}
		}
	}
	if a := s.archives[row]; a != nil {
		a.trimBefore(minAbs)
	}
}

// TrimAfter trims committed entries newer than maxAbs.
func (s *Service) TrimAfter(row, maxAbs int) {
	if row < 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if row >= len(s.rows) {
		return
	}
	s.rows[row].commits.trimAfter(maxAbs)
	if m := s.immutables[row]; m != nil {
		for k := range m {
			if k > maxAbs {
				delete(m, k)
			}
		}
	}
	if a := s.archives[row]; a != nil {
		a.trimAfter(maxAbs)
	}
}

// TrimAfterMutable drops commits newer than maxAbs but preserves immutable
// playback/import history. It stops when the newest remaining entry is
// immutable or maxAbs is reached.
func (s *Service) TrimAfterMutable(row, maxAbs int) {
	if row < 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if row >= len(s.rows) {
		return
	}
	s.rows[row].commits.trimAfterMutable(maxAbs)
	if m := s.immutables[row]; m != nil {
		for k, ce := range m {
			if k > maxAbs && ce.kind != CommitKindPlayback && ce.kind != CommitKindImport {
				delete(m, k)
			}
		}
	}
}

// The pass from abs on is ABANDONED: its Playback/Import commits (sidecar AND archive) move to the superseded store,
// kept but never answered by the live lookup. The ring is cut at abs-1; below abs is untouched. The caller then lowers
// the freeze watermark so the rewound region renders from the predictor.
func (s *Service) RewindTo(row, abs int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if row < 0 || row >= len(s.rows) {
		return
	}
	var sup *rowArchive
	supersede := func(k int, ce commitEntry) {
		if sup == nil {
			sup = s.ensureSupersededLocked(row)
		}
		sup.append(k, ce.val, ce.typ, ce.kind, s.archiveMaxEntries)
	}
	if a := s.archives[row]; a != nil {
		for _, e := range a.takeFrom(abs) {
			supersede(int(e.abs), commitEntry{val: e.val, typ: e.typ, kind: e.kind})
		}
	}
	if rowMap := s.immutables[row]; rowMap != nil {
		for k, ce := range rowMap {
			if k < abs {
				continue
			}
			if ce.kind == CommitKindPlayback || ce.kind == CommitKindImport {
				supersede(k, ce)
			}
			delete(rowMap, k)
		}
	}
	// Immutables were just superseded, so cutting the ring loses no copy.
	s.rows[row].commits.trimAfter(abs - 1)
	// The rewound region is re-simulated and re-recorded as it plays again.
	s.played[row].truncateFrom(abs)
}

// Reconciles seeded/gap commits with the live predictor without truncating history; false outside the retained range.
func (s *Service) ReplaceCommit(row, abs int, val bool, typ model.NodeType, kind CommitKind) bool {
	if row < 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if row >= len(s.rows) {
		return false
	}
	_, _, curKind, curOK := s.valueLocked(row, abs)
	if s.rows[row].commits.replace(abs, val, typ, kind) {
		// So lookups never return a stale playback/import entry after a demotion.
		rowMap := s.immutables[row]
		if kind == CommitKindPlayback || kind == CommitKindImport {
			if s.immutables == nil {
				s.immutables = make(map[int]map[int]commitEntry)
			}
			if rowMap == nil {
				rowMap = make(map[int]commitEntry)
				s.immutables[row] = rowMap
			}
			rowMap[abs] = commitEntry{val: val, typ: typ, kind: kind}
		} else if curOK && (curKind == CommitKindPlayback || curKind == CommitKindImport) && rowMap != nil {
			delete(rowMap, abs)
			if len(rowMap) == 0 {
				delete(s.immutables, row)
			}
		}
		return true
	}

	// Out of the ring: the sidecar entry is still updated or removed, so leaked future playback commits can be demoted.
	if curOK && (curKind == CommitKindPlayback || curKind == CommitKindImport) {
		if rowMap := s.immutables[row]; rowMap != nil {
			if kind == CommitKindPlayback || kind == CommitKindImport {
				rowMap[abs] = commitEntry{val: val, typ: typ, kind: kind}
				return true
			}
			// Archived before the sidecar deletion: it may be the only surviving record of a played beat, and scroll-back must
			// still resolve it.
			if ce, ok := rowMap[abs]; ok {
				s.ensureArchiveLocked(row).append(abs, ce.val, ce.typ, ce.kind, s.archiveMaxEntries)
			}
			delete(rowMap, abs)
			if len(rowMap) == 0 {
				delete(s.immutables, row)
			}
			return true
		}
	}
	return false
}

// ClearRow removes all commits for the row.
func (s *Service) ClearRow(row int) {
	if row < 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if row >= len(s.rows) {
		return
	}
	s.rows[row].commits.clear()
	delete(s.immutables, row)
	delete(s.archives, row)
	delete(s.superseded, row)
	delete(s.played, row)
}

// LastCommitted returns the newest absolute index stored for the row, or -1 if
// empty.
func (s *Service) LastCommitted(row int) int {
	if row < 0 {
		return -1
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if row >= len(s.rows) {
		return -1
	}
	return s.rows[row].commits.last()
}

// LastImmutableAbs returns the newest playback/import commit for the row, or
// -1 if none exist.
func (s *Service) LastImmutableAbs(row int) int {
	if row < 0 {
		return -1
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.immutables == nil {
		return -1
	}
	m := s.immutables[row]
	if len(m) == 0 {
		return -1
	}
	max := -1
	for abs := range m {
		if abs > max {
			max = abs
		}
	}
	return max
}
