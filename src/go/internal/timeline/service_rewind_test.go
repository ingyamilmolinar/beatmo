package timeline

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestRewindToRetainsPostAbsImmutablesAsSuperseded(t *testing.T) {
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
	// 5..9 left the live tiers (that pass was abandoned) but are kept in the superseded store.
	if s.LastImmutableAbs(0) >= 5 {
		t.Fatalf("immutables >= 5 should have been moved out, last=%d", s.LastImmutableAbs(0))
	}
	for abs := 5; abs < 10; abs++ {
		if _, _, _, ok := s.CommittedKind(0, abs); ok {
			t.Fatalf("abs %d of the abandoned pass still answers the live lookup", abs)
		}
	}
	if got := supersededCountForTest(s, 0); got != 5 {
		t.Fatalf("expected the 5 superseded entries retained, got %d", got)
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

// A backward seek ABANDONS the pass after the seek point: kept (full history), but out of the LIVE lookup the row window,
// the played freeze and parity read, or an abandoned commit reads as history for a cell not played in this pass. Hot
// tiers AND the archive.
func TestRewindToSupersededPassIsNotLiveHistory(t *testing.T) {
	s := NewService()
	s.SetArchiveMaxEntries(0)
	const n = immutablesPerRowMax * 3 // ages the oldest third+ into the archive
	for abs := 0; abs < n; abs++ {
		s.RecordCommitKind(0, abs, true, model.NodeTypeRegular, CommitKindPlayback, 64, 64)
	}
	if archivedCountForTest(s, 0) == 0 {
		t.Fatal("setup: nothing aged into the archive")
	}
	const target = 100 // below the archived range's top: archived AND hot entries are superseded
	s.RewindTo(0, target)
	for _, abs := range []int{target, target + 1, immutablesPerRowMax, n - 1} {
		if v, _, k, ok := s.CommittedKind(0, abs); ok {
			t.Fatalf("abs %d (abandoned pass) still answers the live lookup: val=%v kind=%v", abs, v, k)
		}
	}
	if _, _, _, ok := s.CommittedKind(0, target-1); !ok {
		t.Fatal("abs below the seek point must keep its scroll-back history")
	}
	if supersededCountForTest(s, 0) != n-target {
		t.Fatalf("superseded pass must be retained, not dropped: have %d want %d", supersededCountForTest(s, 0), n-target)
	}
	// Once the replay ages out of the hot tiers, the archive must hold it, not the abandoned pass.
	for abs := target; abs < n+immutablesPerRowMax*2; abs++ {
		s.RecordCommitKind(0, abs, false, model.NodeTypeRegular, CommitKindPlayback, 64, 64)
	}
	if v, _, _, ok := s.CommittedKind(0, target+1); !ok || v {
		t.Fatalf("replayed cell must read the replay (false), got ok=%v val=%v", ok, v)
	}
}

func supersededCountForTest(s *Service, row int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.superseded[row].lenForTest()
}
