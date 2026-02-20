package timeline

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func (s *Service) valueLocked(row, abs int) (bool, model.NodeType, CommitKind, bool) {
	if m := s.immutables[row]; m != nil {
		if ce, ok := m[abs]; ok {
			return ce.val, ce.typ, ce.kind, true
		}
	}
	return s.rows[row].commits.value(abs)
}

// Snapshot returns a deep copy of the current segments for the row.
func (s *Service) Snapshot(row int) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if row < 0 || row >= len(s.rows) {
		return Snapshot{}
	}
	src := s.rows[row]
	return Snapshot{
		Offset:    src.offset,
		Past:      cloneBoolSlice(src.past),
		PastMask:  cloneBoolSlice(src.pastMask),
		PastTypes: cloneNodeTypeSlice(src.pastTypes),
		Present:   cloneBoolSlice(src.present),
		Future:    cloneBoolSlice(src.future),
	}
}

func (s *Service) ensureRow(row, windowLen int) *rowState {
	if row < 0 {
		return nil
	}
	if row >= len(s.rows) {
		newRows := make([]rowState, row+1)
		copy(newRows, s.rows)
		s.rows = newRows
	}
	seg := &s.rows[row]
	if windowLen < 0 {
		windowLen = 0
	}
	if seg.windowLen != windowLen {
		seg.windowLen = windowLen
		seg.past = make([]bool, windowLen)
		seg.pastTypes = make([]model.NodeType, windowLen)
		seg.pastMask = make([]bool, windowLen)
		seg.present = make([]bool, windowLen)
		seg.future = make([]bool, windowLen)
		seg.commits.ensureCapacity(windowLen)
	} else {
		if len(seg.past) != windowLen {
			seg.past = make([]bool, windowLen)
		}
		if len(seg.pastTypes) != windowLen {
			seg.pastTypes = make([]model.NodeType, windowLen)
		}
		if len(seg.pastMask) != windowLen {
			seg.pastMask = make([]bool, windowLen)
		}
		if len(seg.present) != windowLen {
			seg.present = make([]bool, windowLen)
		}
		if len(seg.future) != windowLen {
			seg.future = make([]bool, windowLen)
		}
		seg.commits.ensureCapacity(windowLen)
	}
	return seg
}
