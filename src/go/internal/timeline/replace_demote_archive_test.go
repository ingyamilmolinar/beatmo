package timeline

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestReplaceCommitDemoteArchivesNoRingImmutable guards the immutability
// contract for a playback/import commit that lives only in the immutables
// sidecar (the commit ring aged/gapped it out). Demoting such a commit to a
// mutable kind must NOT silently drop it: the value has to migrate to the cold
// archive so scroll-back still resolves it.
//
// Regression: the long-session canary lost row-0 abs=8192 because
// refreshDrumRow's "demote leaked future immutable" pass called ReplaceCommit
// on a seeded playback commit the ring never held; the noring branch deleted it
// with no archival, so once the playhead passed it in a single Update step it
// was gone from every read tier.
func TestReplaceCommitDemoteArchivesNoRingImmutable(t *testing.T) {
	s := NewService()
	const windowLen = 64
	const farAbs = 8192

	// Seed a near-base commit so the ring has a small base far below farAbs.
	s.RecordCommitKind(0, 1, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, windowLen)
	// Seed a playback commit far ahead of the ring base. The gap exceeds the
	// ring's hard ceiling (capMax = windowLen*commitRingCapMultiplier), so the
	// ring append bails out and the entry lives ONLY in the immutables sidecar.
	s.RecordCommitKind(0, farAbs, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, windowLen)

	if tier := s.TierForTest(0, farAbs); tier != "immutables" {
		t.Fatalf("precondition: abs=%d should be immutables-only, got tier=%s", farAbs, tier)
	}

	// Demote the far playback commit to a mutable kind, exactly as the
	// refresh "leaked future immutable" pass does.
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
