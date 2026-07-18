package timeline

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestRewindToArchivesPostAbsImmutables(t *testing.T) {
	s := NewService()
	// Record immutable playback hits at abs 0..9 on row 0.
	for abs := 0; abs < 10; abs++ {
		s.RecordCommitKind(0, abs, abs%2 == 0, model.NodeTypeRegular, CommitKindPlayback, 64, 64)
	}
	// Rewind to abs 5: 5..9 immutables move to archive, 0..4 stay live.
	s.RewindTo(0, 5)

	// 0..4 still resolve via the live tiers.
	for abs := 0; abs < 5; abs++ {
		if _, _, _, ok := s.CommittedKind(0, abs); !ok {
			t.Fatalf("abs %d below seek point should stay committed", abs)
		}
	}
	// 5..9 are no longer in immutables/ring, but remain retrievable in the
	// archive tier (history is archived, not dropped).
	if s.LastImmutableAbs(0) >= 5 {
		t.Fatalf("immutables >= 5 should have been archived, last=%d", s.LastImmutableAbs(0))
	}
	// Archive retains at least one of the superseded entries.
	if got := archivedCountForTest(s, 0); got == 0 {
		t.Fatal("expected archived entries after rewind, got 0")
	}
}

func TestRewindToPreservesBelowAbs(t *testing.T) {
	s := NewService()
	for abs := 0; abs < 6; abs++ {
		s.RecordCommitKind(0, abs, true, model.NodeTypeRegular, CommitKindPlayback, 64, 64)
	}
	s.RewindTo(0, 3)
	val, _, kind, ok := s.CommittedKind(0, 2)
	if !ok || !val || kind != CommitKindPlayback {
		t.Fatalf("abs 2 must be preserved as playback true, got ok=%v val=%v kind=%v", ok, val, kind)
	}
}

// archivedCountForTest reports how many entries the row's cold archive holds.
func archivedCountForTest(s *Service, row int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	arch := s.archives[row]
	if arch == nil {
		return 0
	}
	return arch.lenForTest()
}
