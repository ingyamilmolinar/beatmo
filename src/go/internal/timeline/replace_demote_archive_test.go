package timeline

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Demoting a Playback/Import commit that only the immutables sidecar holds (aged out of the ring) must migrate it to the
// cold archive, never drop it: scroll-back has to resolve it.
func TestReplaceCommitDemoteArchivesNoRingImmutable(t *testing.T) {
	s := NewService()
	const windowLen = 64
	const farAbs = 8192

	// Seed a near-base commit so the ring has a small base far below farAbs.
	s.RecordCommitKind(0, 1, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, windowLen)
	// The gap exceeds the ring's ceiling, so the ring append bails and the entry lives ONLY in the sidecar.
	s.RecordCommitKind(0, farAbs, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, windowLen)

	if tier := s.TierForTest(0, farAbs); tier != "immutables" {
		t.Fatalf("precondition: abs=%d should be immutables-only, got tier=%s", farAbs, tier)
	}

	// As refreshDrumRow's "demote leaked future immutable" pass does.
	if !s.ReplaceCommit(0, farAbs, true, model.NodeTypeRegular, CommitKindReleased) {
		t.Fatalf("ReplaceCommit returned false for abs=%d", farAbs)
	}

	// The played fact must still resolve — now via the cold archive.
	val, typ, kind, ok := s.CommittedKind(0, farAbs)
	if !ok {
		t.Fatalf("abs=%d lost after demotion (tier=%s) — immutable playback dropped", farAbs, s.TierForTest(0, farAbs))
	}
	if kind != CommitKindPlayback {
		t.Errorf("abs=%d kind=%v want Playback (archive must retain provenance)", farAbs, kind)
	}
	if !val || typ != model.NodeTypeRegular {
		t.Errorf("abs=%d val=%v typ=%v want true/Regular", farAbs, val, typ)
	}
	if got := s.ArchiveLenForTest(0); got == 0 {
		t.Errorf("archive empty after demotion; expected the demoted immutable to be preserved")
	}
}
