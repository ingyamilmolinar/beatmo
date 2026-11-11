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

// RewindTo repositions row `row` for a backward seek to absolute subdivision
// `abs`. The pass from abs on is ABANDONED — it is about to be replayed — so
// every Playback/Import commit with key >= abs (hot sidecar AND entries that
// had already aged into the scroll-back archive) moves to the superseded store:
// retained (full-history rule), but never answered by the live lookup, which
// must read only the pass being played. The mutable/ring tier is cut at abs-1
// and everything below abs is left untouched. After a rewind the caller lowers
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
	// Hard-cut the ring at abs-1 (drops any ring entries >= abs, mutable or not;
	// immutables were just superseded, so no copy is lost).
	s.rows[row].commits.trimAfter(abs - 1)
	// The rewound region is re-simulated and re-recorded as it plays again.
	s.played[row].truncateFrom(abs)
}

// ReplaceCommit overwrites an existing commit at abs (if present). This is
// used to reconcile seeded/gap commits with the live predictor without
// truncating history. Returns false if abs is outside the retained range.
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
		// Keep the immutable sidecar in sync so lookups do not return stale
		// playback/import entries after a demotion to Released/Seeded/etc.
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

	// The ring no longer retains this absolute index. If the immutable sidecar
	// contains the entry, still honor the replacement semantics by updating or
	// deleting the sidecar so callers can demote leaked future playback commits.
	if curOK && (curKind == CommitKindPlayback || curKind == CommitKindImport) {
		if rowMap := s.immutables[row]; rowMap != nil {
			if kind == CommitKindPlayback || kind == CommitKindImport {
				rowMap[abs] = commitEntry{val: val, typ: typ, kind: kind}
				return true
			}
			// The ring no longer retains this abs, so demoting the immutable
			// sidecar entry by deletion would drop the only surviving record of
			// this played/imported beat. Migrate it to the cold archive first so
			// scroll-back still resolves it — mirrors pruneImmutablesRowLocked and
			// upholds the playback/import immutability contract even when a live
			// edit demotes a leaked future commit that has aged out of the ring.
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
