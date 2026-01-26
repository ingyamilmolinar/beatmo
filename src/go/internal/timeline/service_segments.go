package timeline

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
)
// UpdateRowSegments refreshes the cached present/future slices for a row using
// the provided window and steps. The callback executes while holding the
// service lock; callers must avoid re-entering the service from within fn.
func (s *Service) UpdateRowSegments(row, offset, windowLen, capacityHint int, steps []bool, fn func(SegmentsView)) {
	if row < 0 || fn == nil {
		return
	}
	if windowLen < 0 {
		windowLen = 0
	}
	if capacityHint < windowLen {
		capacityHint = windowLen
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seg := s.ensureRow(row, windowLen)
	if seg == nil {
		fn(SegmentsView{})
		return
	}
	seg.commits.ensureCapacity(capacityHint)
	seg.offset = offset
	for i := 0; i < windowLen; i++ {
		abs := offset + i
		seg.past[i] = false
		seg.pastMask[i] = false
		seg.pastTypes[i] = model.NodeTypeInvisible
		if val, typ, kind, ok := s.valueLocked(row, abs); ok {
			seg.past[i] = val
			seg.pastMask[i] = (kind == CommitKindPlayback || kind == CommitKindImport)
			seg.pastTypes[i] = typ
		}
		step := false
		if i < len(steps) {
			step = steps[i]
		}
		seg.present[i] = step
		seg.future[i] = step
	}
	view := SegmentsView{
		Offset:    seg.offset,
		Past:      seg.past,
		PastMask:  seg.pastMask,
		PastTypes: seg.pastTypes,
		Present:   seg.present,
		Future:    seg.future,
	}
	fn(view)
}

// Committed reports whether a past entry exists for the given absolute index.
func (s *Service) Committed(row, abs int) (bool, model.NodeType, bool) {
	if row < 0 {
		return false, model.NodeTypeInvisible, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if row >= len(s.rows) {
		return false, model.NodeTypeInvisible, false
	}
	val, typ, _, ok := s.valueLocked(row, abs)
	return val, typ, ok
}

// CommittedKind returns the stored value, type, provenance, and ok flag.
func (s *Service) CommittedKind(row, abs int) (bool, model.NodeType, CommitKind, bool) {
	if row < 0 {
		return false, model.NodeTypeInvisible, CommitKindPlayback, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if row >= len(s.rows) {
		return false, model.NodeTypeInvisible, CommitKindPlayback, false
	}
	return s.valueLocked(row, abs)
}

// CommittedRange returns the [start,end] absolute indices currently retained
// for the row. The third return value is false when no commits exist.
func (s *Service) CommittedRange(row int) (int, int, bool) {
	if row < 0 {
		return 0, 0, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if row >= len(s.rows) {
		return 0, 0, false
	}
	r := s.rows[row].commits
	if r.size == 0 {
		return 0, 0, false
	}
	start := r.base
	end := r.base + r.size - 1
	return start, end, true
}

