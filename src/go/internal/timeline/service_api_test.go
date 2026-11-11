package timeline

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestServiceRecordCommitImmutableAndRange exercises the canonical write
// path: RecordCommit lays down a Playback entry, CommittedRange reports
// the [base, last] window, Committed reads it back. This is the entry
// point that ~every UI write to the timeline funnels through, but it had
// no direct unit test.
func TestServiceRecordCommitImmutableAndRange(t *testing.T) {
	s := NewService()
	const row = 0
	for abs := 0; abs < 8; abs++ {
		s.RecordCommit(row, abs, abs%2 == 0, model.NodeTypeRegular, 16, 16)
	}
	start, end, ok := s.CommittedRange(row)
	if !ok || start != 0 || end != 7 {
		t.Errorf("CommittedRange = (%d,%d,%v), want (0,7,true)", start, end, ok)
	}
	for abs := 0; abs < 8; abs++ {
		val, typ, ok := s.Committed(row, abs)
		if !ok {
			t.Errorf("Committed(abs=%d) ok=false", abs)
			continue
		}
		if want := abs%2 == 0; val != want {
			t.Errorf("Committed(abs=%d) val=%v, want %v", abs, val, want)
		}
		if typ != model.NodeTypeRegular {
			t.Errorf("Committed(abs=%d) typ=%v, want NodeTypeRegular", abs, typ)
		}
	}
}

// TestServiceRecordCommitPreservesImmutables — once Playback has been
// recorded, a subsequent attempt to overwrite with any kind must be a
// no-op. This is the "past is immutable" contract.
func TestServiceRecordCommitPreservesImmutables(t *testing.T) {
	s := NewService()
	s.RecordCommit(0, 5, true, model.NodeTypeRegular, 16, 16)
	// Try to overwrite as Seeded — must NOT take effect.
	s.RecordCommitKind(0, 5, false, model.NodeTypeMute, CommitKindSeeded, 16, 16)
	val, typ, kind, ok := s.CommittedKind(0, 5)
	if !ok {
		t.Fatal("entry missing after immutable preserve")
	}
	if !val || typ != model.NodeTypeRegular || kind != CommitKindPlayback {
		t.Errorf("immutable was overwritten: val=%v typ=%v kind=%v; want (true, Regular, Playback)", val, typ, kind)
	}
}

// TestServiceReplaceCommitMutable demonstrates the inverse: a mutable
// (Seeded/GapPad) entry can be replaced in place.
func TestServiceReplaceCommitMutable(t *testing.T) {
	s := NewService()
	s.RecordCommitKind(0, 3, true, model.NodeTypeRegular, CommitKindSeeded, 16, 16)
	if ok := s.ReplaceCommit(0, 3, false, model.NodeTypeMute, CommitKindSeeded); !ok {
		t.Fatal("ReplaceCommit returned false; want true")
	}
	val, typ, kind, ok := s.CommittedKind(0, 3)
	if !ok || val != false || typ != model.NodeTypeMute || kind != CommitKindSeeded {
		t.Errorf("after replace: val=%v typ=%v kind=%v ok=%v; want (false, Mute, Seeded, true)", val, typ, kind, ok)
	}
}

// TestServiceReplaceCommitOutsideRangeFalse — replacing an abs that
// isn't retained anywhere (ring or immutables) must return false so the
// caller knows the write was rejected.
func TestServiceReplaceCommitOutsideRangeFalse(t *testing.T) {
	s := NewService()
	s.RecordCommit(0, 5, true, model.NodeTypeRegular, 16, 16)
	if ok := s.ReplaceCommit(0, 99, false, model.NodeTypeMute, CommitKindSeeded); ok {
		t.Error("ReplaceCommit(abs=99) returned true; expected false (out of range)")
	}
	if ok := s.ReplaceCommit(-1, 0, false, model.NodeTypeMute, CommitKindSeeded); ok {
		t.Error("ReplaceCommit(row=-1) returned true; expected false")
	}
}

// TestServiceTrimBefore drops entries with abs < minAbs and re-bases the
// ring. Also propagates to the cold archive and the immutables sidecar.
func TestServiceTrimBefore(t *testing.T) {
	s := NewService()
	for abs := 0; abs < 12; abs++ {
		s.RecordCommit(0, abs, true, model.NodeTypeRegular, 16, 16)
	}
	s.TrimBefore(0, 8)
	start, end, ok := s.CommittedRange(0)
	if !ok || start != 8 || end != 11 {
		t.Errorf("post-trim range = (%d,%d,%v), want (8,11,true)", start, end, ok)
	}
	if _, _, ok := s.Committed(0, 5); ok {
		t.Error("Committed(abs=5) still ok after TrimBefore(8)")
	}
	if _, _, ok := s.Committed(0, 8); !ok {
		t.Error("Committed(abs=8) gone after TrimBefore(8)")
	}
}

// TestServiceTrimAfter mirrors TrimBefore but at the high end.
func TestServiceTrimAfter(t *testing.T) {
	s := NewService()
	for abs := 0; abs < 12; abs++ {
		s.RecordCommit(0, abs, true, model.NodeTypeRegular, 16, 16)
	}
	s.TrimAfter(0, 5)
	start, end, ok := s.CommittedRange(0)
	if !ok || start != 0 || end != 5 {
		t.Errorf("post-trim range = (%d,%d,%v), want (0,5,true)", start, end, ok)
	}
	if _, _, ok := s.Committed(0, 9); ok {
		t.Error("Committed(abs=9) still ok after TrimAfter(5)")
	}
}

// TestServiceTrimAfterMutablePreservesImmutable — the mutable variant
// must keep Playback/Import entries above maxAbs untouched.
func TestServiceTrimAfterMutablePreservesImmutable(t *testing.T) {
	s := NewService()
	// abs 0..4 = Seeded (mutable), abs 5..7 = Playback (immutable).
	for abs := 0; abs < 5; abs++ {
		s.RecordCommitKind(0, abs, true, model.NodeTypeRegular, CommitKindSeeded, 16, 16)
	}
	for abs := 5; abs < 8; abs++ {
		s.RecordCommit(0, abs, true, model.NodeTypeRegular, 16, 16)
	}
	s.TrimAfterMutable(0, 4)
	// Immutables ≥5 must survive in the sidecar even though the ring
	// trimAfterMutable bails at the first immutable entry it sees.
	for abs := 5; abs < 8; abs++ {
		if _, _, ok := s.Committed(0, abs); !ok {
			t.Errorf("immutable abs=%d lost after TrimAfterMutable", abs)
		}
	}
}

// TestServiceClearRow drops all state for a row (ring + sidecar +
// archive). After ClearRow, every read for that row must be empty.
func TestServiceClearRow(t *testing.T) {
	s := NewService()
	for abs := 0; abs < 8; abs++ {
		s.RecordCommit(0, abs, true, model.NodeTypeRegular, 16, 16)
	}
	s.ClearRow(0)
	if _, _, ok := s.CommittedRange(0); ok {
		t.Error("CommittedRange ok=true after ClearRow")
	}
	if s.HasImmutable(0) {
		t.Error("HasImmutable=true after ClearRow")
	}
	if abs := s.LastCommitted(0); abs != -1 {
		t.Errorf("LastCommitted=%d after ClearRow, want -1", abs)
	}
	if abs := s.LastImmutableAbs(0); abs != -1 {
		t.Errorf("LastImmutableAbs=%d after ClearRow, want -1", abs)
	}
}

// TestServiceLastCommittedAndLastImmutable distinguishes the two: ring
// tail vs the largest Playback/Import abs in the sidecar.
func TestServiceLastCommittedAndLastImmutable(t *testing.T) {
	s := NewService()
	s.RecordCommit(0, 1, true, model.NodeTypeRegular, 16, 16)
	s.RecordCommit(0, 7, true, model.NodeTypeRegular, 16, 16)
	if got := s.LastCommitted(0); got != 7 {
		t.Errorf("LastCommitted=%d, want 7", got)
	}
	if got := s.LastImmutableAbs(0); got != 7 {
		t.Errorf("LastImmutableAbs=%d, want 7", got)
	}

	// A Seeded entry at abs=9 should bump LastCommitted but NOT
	// LastImmutableAbs (the sidecar tracks only immutables).
	s.RecordCommitKind(0, 9, true, model.NodeTypeRegular, CommitKindSeeded, 16, 16)
	if got := s.LastCommitted(0); got != 9 {
		t.Errorf("after Seeded: LastCommitted=%d, want 9", got)
	}
	if got := s.LastImmutableAbs(0); got != 7 {
		t.Errorf("after Seeded: LastImmutableAbs=%d, want 7", got)
	}
}

// TestServiceHasImmutable is the cheap "is there anything immutable for
// this row" predicate used by hot overlay paths.
func TestServiceHasImmutable(t *testing.T) {
	s := NewService()
	if s.HasImmutable(0) {
		t.Error("HasImmutable on empty row should be false")
	}
	s.RecordCommitKind(0, 0, true, model.NodeTypeRegular, CommitKindSeeded, 16, 16)
	if s.HasImmutable(0) {
		t.Error("HasImmutable should be false for a row with only Seeded commits")
	}
	s.RecordCommit(0, 1, true, model.NodeTypeRegular, 16, 16)
	if !s.HasImmutable(0) {
		t.Error("HasImmutable should be true after a Playback commit")
	}
}

// TestServiceSnapshotReturnsDeepCopy — the returned slices must not
// share storage with the live segments, so callers can hold them across
// subsequent service calls without races / mutation.
func TestServiceSnapshotReturnsDeepCopy(t *testing.T) {
	s := NewService()
	s.UpdateRowSegments(0, 0, 4, 4, []bool{true, false, true, false}, func(SegmentsView) {})
	snap := s.Snapshot(0)
	if len(snap.Present) != 4 {
		t.Fatalf("Snapshot.Present len=%d, want 4", len(snap.Present))
	}
	want := snap.Present[0]
	snap.Present[0] = !want
	snap2 := s.Snapshot(0)
	if snap2.Present[0] != want {
		t.Errorf("mutating the snapshot leaked into the service: now %v, want %v", snap2.Present[0], want)
	}
}

// TestServiceSnapshotOutOfRange returns the zero Snapshot for unknown
// rows so callers don't have to guard upstream.
func TestServiceSnapshotOutOfRange(t *testing.T) {
	s := NewService()
	if snap := s.Snapshot(-1); len(snap.Past) != 0 || len(snap.Present) != 0 {
		t.Error("Snapshot(-1) should be empty")
	}
	if snap := s.Snapshot(99); len(snap.Past) != 0 || len(snap.Present) != 0 {
		t.Error("Snapshot(99) should be empty")
	}
}

// TestServiceUpdateRowSegmentsRunsCallbackWithLiveSlices — the callback
// API hands the caller live (not copied) slices for hot-path rendering.
// We verify the windowing math: past gets values from the commit ring,
// present/future come from the supplied steps argument.
func TestServiceUpdateRowSegmentsRunsCallbackWithLiveSlices(t *testing.T) {
	s := NewService()
	s.RecordCommit(0, 1, true, model.NodeTypeRegular, 4, 4)

	steps := []bool{false, true, false, true}
	called := false
	s.UpdateRowSegments(0, 0, 4, 4, steps, func(view SegmentsView) {
		called = true
		if view.Offset != 0 {
			t.Errorf("view.Offset = %d, want 0", view.Offset)
		}
		if len(view.Present) != 4 || len(view.Future) != 4 {
			t.Errorf("view sizes: present=%d future=%d, want 4 each", len(view.Present), len(view.Future))
		}
		// past[1] should match the Playback commit we just laid down.
		if !view.Past[1] {
			t.Errorf("view.Past[1]=false, want true (from RecordCommit at abs=1)")
		}
		if !view.PastMask[1] {
			t.Errorf("view.PastMask[1]=false, want true (Playback is masked)")
		}
		// present == steps argument verbatim.
		for i, got := range view.Present {
			if got != steps[i] {
				t.Errorf("view.Present[%d]=%v, want %v", i, got, steps[i])
			}
		}
	})
	if !called {
		t.Error("UpdateRowSegments did not invoke its callback")
	}
}

// TestServiceUpdateRowSegmentsHandlesNilFn and negative row.
func TestServiceUpdateRowSegmentsHandlesNilFn(t *testing.T) {
	s := NewService()
	// nil fn — must not panic.
	s.UpdateRowSegments(0, 0, 4, 4, nil, nil)
	// negative row — also a no-op.
	s.UpdateRowSegments(-3, 0, 4, 4, nil, func(SegmentsView) {
		t.Error("callback should not fire for row < 0")
	})
}

// TestServiceSeedFromWindow lays down speculative history; subsequent
// reads must surface the seeded values without polluting the immutable
// sidecar.
func TestServiceSeedFromWindow(t *testing.T) {
	s := NewService()
	steps := []bool{true, false, true, false, true}
	types := []model.NodeType{
		model.NodeTypeRegular, model.NodeTypeMute,
		model.NodeTypeRegular, model.NodeTypeMute, model.NodeTypeRegular,
	}
	s.SeedFromWindow(0, 0, 4, steps, types, CommitKindSeeded, 8, 8)
	for abs := 0; abs <= 4; abs++ {
		val, typ, kind, ok := s.CommittedKind(0, abs)
		if !ok {
			t.Errorf("CommittedKind(abs=%d) ok=false after Seed", abs)
			continue
		}
		if val != steps[abs] || typ != types[abs] || kind != CommitKindSeeded {
			t.Errorf("CommittedKind(abs=%d) = (%v,%v,%v); want (%v,%v,Seeded)", abs, val, typ, kind, steps[abs], types[abs])
		}
	}
	if s.HasImmutable(0) {
		t.Error("HasImmutable=true after Seeded-only writes")
	}
}

// TestServiceSeedFromWindowRespectsImmutable — Playback entries in the
// sidecar must not be overwritten by a seeded reconcile.
func TestServiceSeedFromWindowRespectsImmutable(t *testing.T) {
	s := NewService()
	s.RecordCommit(0, 2, true, model.NodeTypeRegular, 8, 8)
	s.SeedFromWindow(0, 0, 4,
		[]bool{false, false, false, false, false},
		[]model.NodeType{model.NodeTypeMute, model.NodeTypeMute, model.NodeTypeMute, model.NodeTypeMute, model.NodeTypeMute},
		CommitKindSeeded, 8, 8)
	val, typ, kind, ok := s.CommittedKind(0, 2)
	if !ok || !val || typ != model.NodeTypeRegular || kind != CommitKindPlayback {
		t.Errorf("immutable abs=2 was overwritten by Seed: val=%v typ=%v kind=%v ok=%v", val, typ, kind, ok)
	}
}

// TestServiceTrimAfterPathChangeReport asserts the report and the
// preservation contract: cutoff=N drops mutable entries with abs > N-1,
// but keeps the immutable lastImm.
func TestServiceTrimAfterPathChangeReport(t *testing.T) {
	s := NewService()
	s.RecordCommit(0, 1, true, model.NodeTypeRegular, 8, 8)
	s.RecordCommit(0, 2, true, model.NodeTypeRegular, 8, 8)
	s.RecordCommitKind(0, 5, true, model.NodeTypeRegular, CommitKindSeeded, 8, 8)

	report := s.TrimAfterPathChange(0, 3 /* cutoffExclusive */, 1 /* freezeLimit */)
	if report.LastImmutable != 2 {
		t.Errorf("report.LastImmutable=%d, want 2", report.LastImmutable)
	}
	// Mutable Seeded at abs=5 must have been pruned (cutoff−1=2 ≥
	// freezeLimit and ≥ lastImm, so maxKeep=2).
	if _, _, ok := s.Committed(0, 5); ok {
		t.Error("Seeded at abs=5 survived TrimAfterPathChange(cutoff=3)")
	}
	if _, _, ok := s.Committed(0, 2); !ok {
		t.Error("immutable at abs=2 was dropped")
	}
}

// TestServiceSetArchiveMaxEntriesShrinksCeiling — the ceiling drives
// pruneImmutablesRow → archive eviction. Setting a low ceiling and
// pushing past immutablesPerRowMax exercises the cold-archive migration
// path; archive then capped by the new ceiling.
func TestServiceSetArchiveMaxEntriesShrinksCeiling(t *testing.T) {
	s := NewService()
	s.SetArchiveMaxEntries(8)
	// Push enough immutables to overflow immutablesPerRowMax (=1024) so
	// pruneImmutablesRow fires the migration into the cold archive.
	const total = immutablesPerRowMax + 32
	for abs := 0; abs < total; abs++ {
		s.RecordCommit(0, abs, true, model.NodeTypeRegular, 2048, 2048)
	}
	got := s.ArchiveLenForTest(0)
	if got > 8+archiveEvictChunk {
		t.Errorf("archive grew to %d after SetArchiveMaxEntries(8); want ≤ %d (cap + 1 evict chunk)", got, 8+archiveEvictChunk)
	}
	// Reads for the oldest abs that aged out of the sidecar but landed
	// in the archive must still resolve via the archive tier — unless
	// the eviction chunk dropped them. We pick abs=total-1 which we
	// just wrote; it should still be live in the ring or sidecar.
	if _, _, ok := s.Committed(0, total-1); !ok {
		t.Error("most-recent commit no longer reachable")
	}
}

// TestServiceRingLenForTestAndArchiveLenForTest are surfaced for soak
// regressions; cover them so a refactor that breaks the seam fails
// loudly.
func TestServiceRingLenForTestAndArchiveLenForTest(t *testing.T) {
	s := NewService()
	if ring, cap, imm := s.RingLenForTest(0); ring != 0 || cap != 0 || imm != 0 {
		t.Errorf("empty row RingLenForTest = (%d,%d,%d), want (0,0,0)", ring, cap, imm)
	}
	s.RecordCommit(0, 0, true, model.NodeTypeRegular, 4, 4)
	if ring, _, imm := s.RingLenForTest(0); ring != 1 || imm != 1 {
		t.Errorf("after 1 commit: ring=%d imm=%d, want 1/1", ring, imm)
	}
	if got := s.ArchiveLenForTest(0); got != 0 {
		t.Errorf("ArchiveLenForTest after 1 commit = %d, want 0", got)
	}
}

// TestServiceNegativeRowGuards — every public entry point must safely
// handle row<0 without panicking or mutating state.
func TestServiceNegativeRowGuards(t *testing.T) {
	s := NewService()
	s.RecordCommit(-1, 0, true, model.NodeTypeRegular, 4, 4)
	s.RecordCommitKind(-1, 0, true, model.NodeTypeRegular, CommitKindSeeded, 4, 4)
	s.SeedFromWindow(-1, 0, 4, nil, nil, CommitKindSeeded, 4, 4)
	s.UpdateRowSegments(-1, 0, 4, 4, nil, nil)
	s.TrimBefore(-1, 0)
	s.TrimAfter(-1, 0)
	s.TrimAfterMutable(-1, 0)
	s.ClearRow(-1)
	if ok := s.ReplaceCommit(-1, 0, false, model.NodeTypeRegular, CommitKindSeeded); ok {
		t.Error("ReplaceCommit(-1) returned true")
	}
	if got := s.LastCommitted(-1); got != -1 {
		t.Errorf("LastCommitted(-1)=%d, want -1", got)
	}
	if got := s.LastImmutableAbs(-1); got != -1 {
		t.Errorf("LastImmutableAbs(-1)=%d, want -1", got)
	}
	if v, t1, ok := s.Committed(-1, 0); ok || v || t1 != model.NodeTypeInvisible {
		t.Errorf("Committed(-1) returned non-empty: (%v,%v,%v)", v, t1, ok)
	}
	if _, _, ok := s.CommittedRange(-1); ok {
		t.Error("CommittedRange(-1) ok=true")
	}
}
