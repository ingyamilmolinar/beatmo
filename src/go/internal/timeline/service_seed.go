package timeline

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
)
// SeedFromWindow writes commits for the range [offset, pastEnd] using the
// provided rendered window values. It is intended for path-change reconciliation
// so already-rendered history remains stable even when the beat path shape
// changes (e.g., invisible gaps become explicit nodes).
//
// Playback/import commits are never overwritten.
func (s *Service) SeedFromWindow(row, offset, pastEnd int, steps []bool, types []model.NodeType, kind CommitKind, windowLen, capacity int) {
	if row < 0 {
		return
	}
	if pastEnd < offset {
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
	seg.commits.ensureCapacity(capacity)

	for abs := offset; abs <= pastEnd; abs++ {
		idx := abs - offset
		val := false
		if idx >= 0 && idx < len(steps) {
			val = steps[idx]
		}
		typ := model.NodeTypeInvisible
		if idx >= 0 && idx < len(types) {
			typ = types[idx]
		}

		// Never overwrite immutable entries retained in the sidecar.
		if m := s.immutables[row]; m != nil {
			if _, ok := m[abs]; ok {
				continue
			}
		}

		if _, _, existingKind, ok := seg.commits.value(abs); ok {
			if existingKind == CommitKindPlayback || existingKind == CommitKindImport {
				continue
			}
			seg.commits.replace(abs, val, typ, kind)
		} else {
			seg.commits.append(abs, val, typ, kind)
		}

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
}

// TrimAfterPathChange trims speculative commits after a path edit, preserving
// immutable playback/import history and any frozen range provided by callers.
//
// The caller supplies cutoffExclusive (typically the row's next beat index) and
// freezeLimit (a UI-maintained ceiling). The service computes maxKeep as:
//
//	max(cutoffExclusive-1, freezeLimit, LastImmutableAbs(row))
//
// Then it trims mutable commits strictly above maxKeep.
func (s *Service) TrimAfterPathChange(row int, cutoffExclusive int, freezeLimit int) PathChangeTrimReport {
	report := PathChangeTrimReport{MaxKeep: -1, LastImmutable: -1}
	if row < 0 {
		return report
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	lastImm := -1
	if s.immutables != nil {
		if m := s.immutables[row]; m != nil {
			for abs, ce := range m {
				if ce.kind != CommitKindPlayback && ce.kind != CommitKindImport {
					continue
				}
				if abs > lastImm {
					lastImm = abs
				}
			}
		}
	}

	maxKeep := cutoffExclusive - 1
	if maxKeep < -1 {
		maxKeep = -1
	}
	if lastImm > maxKeep {
		maxKeep = lastImm
	}
	if freezeLimit > maxKeep {
		maxKeep = freezeLimit
	}

	if row < len(s.rows) {
		s.rows[row].commits.trimAfterMutable(maxKeep)
		// immutables is expected to hold only playback/import entries; nothing
		// to prune here beyond honoring TrimAfterMutable semantics.
	}

	report.MaxKeep = maxKeep
	report.LastImmutable = lastImm
	return report
}

// HasImmutable reports whether the row currently holds any playback/import
// commits. Useful for short-circuiting overlay work in hot paths.
func (s *Service) HasImmutable(row int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.immutables == nil {
		return false
	}
	m := s.immutables[row]
	return len(m) > 0
}

