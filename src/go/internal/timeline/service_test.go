package timeline

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func TestRecordCommitKindStoresProvenance(t *testing.T) {
	s := NewService()
	s.RecordCommitKind(0, 0, true, 1, CommitKindSeeded, 4, 4)
	val, typ, kind, ok := s.CommittedKind(0, 0)
	t.Logf("kinds=%v", s.rows[0].commits.kinds)
	if !ok {
		t.Fatalf("expected commit present")
	}
	if !val || typ != 1 {
		t.Fatalf("unexpected commit payload val=%v typ=%v", val, typ)
	}
	if kind != CommitKindSeeded {
		t.Fatalf("expected kind Seeded got %v", kind)
	}
}

func TestReplaceCommitForSeeded(t *testing.T) {
	s := NewService()
	s.RecordCommitKind(0, 4, false, 1, CommitKindSeeded, 8, 8)
	if !s.ReplaceCommit(0, 4, true, 1, CommitKindReleased) {
		t.Fatalf("replace failed")
	}
	val, _, kind, ok := s.CommittedKind(0, 4)
	if !ok || !val {
		t.Fatalf("expected updated commit val=true ok=%v", ok)
	}
	if kind != CommitKindReleased {
		t.Fatalf("expected kind Released got %v", kind)
	}
}

func TestGapPadKind(t *testing.T) {
	s := NewService()
	s.RecordCommit(0, 0, true, 1, 8, 8)
	s.RecordCommit(0, 2, true, 1, 8, 8)
	_, _, kind, ok := s.CommittedKind(0, 1)
	if !ok {
		t.Fatalf("expected padded commit at gap")
	}
	if kind != CommitKindGapPad {
		t.Fatalf("expected gap pad kind got %v", kind)
	}
}

func TestRecordCommitKindDoesNotOverwriteImmutable(t *testing.T) {
	s := NewService()
	windowLen := 8
	capacity := 8

	s.RecordCommitKind(0, 1, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, capacity)
	s.RecordCommitKind(0, 1, false, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)

	val, _, kind, ok := s.CommittedKind(0, 1)
	if !ok {
		t.Fatalf("expected immutable commit retained")
	}
	if !val || kind != CommitKindPlayback {
		t.Fatalf("expected playback commit preserved; val=%v kind=%v", val, kind)
	}
}

func TestLastImmutableAbsUsesSidecar(t *testing.T) {
	s := NewService()
	s.RecordCommitKind(0, 1, true, model.NodeTypeRegular, CommitKindSeeded, 8, 8)
	s.RecordCommitKind(0, 3, true, model.NodeTypeRegular, CommitKindPlayback, 8, 8)
	s.RecordCommitKind(0, 6, true, model.NodeTypeRegular, CommitKindImport, 8, 8)
	if got := s.LastImmutableAbs(0); got != 6 {
		t.Fatalf("expected last immutable=6 got %d", got)
	}
}

func TestSeedFromWindowDoesNotOverwritePlaybackImport(t *testing.T) {
	s := NewService()
	windowLen := 8
	capacity := 8

	// Existing immutable commit at abs=2 should never be overwritten by seeding.
	s.RecordCommitKind(0, 2, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, capacity)

	steps := []bool{false, false, false, false, false, false, false, false}
	types := make([]model.NodeType, windowLen)
	for i := range types {
		types[i] = model.NodeTypeRegular
	}

	s.SeedFromWindow(0, 0, 4, steps, types, CommitKindSeeded, windowLen, capacity)

	val, _, kind, ok := s.CommittedKind(0, 2)
	if !ok {
		t.Fatalf("expected commit at abs=2 present")
	}
	if kind != CommitKindPlayback {
		t.Fatalf("expected kind Playback at abs=2 got %v", kind)
	}
	if !val {
		t.Fatalf("expected immutable value preserved at abs=2")
	}

	// Seeded commits should exist elsewhere.
	if _, _, kind, ok := s.CommittedKind(0, 3); !ok || kind != CommitKindSeeded {
		t.Fatalf("expected seeded commit at abs=3; ok=%v kind=%v", ok, kind)
	}
}

func TestSeedFromWindowUsesDefaultsWhenSlicesShort(t *testing.T) {
	s := NewService()
	windowLen := 4
	capacity := 4

	steps := []bool{true, false}
	types := []model.NodeType{model.NodeTypeRegular}
	s.SeedFromWindow(0, 0, 3, steps, types, CommitKindSeeded, windowLen, capacity)

	// abs=0 uses provided step/type.
	if val, typ, _, ok := s.CommittedKind(0, 0); !ok || !val || typ != model.NodeTypeRegular {
		t.Fatalf("expected seeded abs=0 from slices; val=%v typ=%v ok=%v", val, typ, ok)
	}
	// abs=2/3 fall back to false/invisible.
	for _, abs := range []int{2, 3} {
		val, typ, _, ok := s.CommittedKind(0, abs)
		if !ok {
			t.Fatalf("expected seeded commit at abs=%d", abs)
		}
		if val || typ != model.NodeTypeInvisible {
			t.Fatalf("expected default false/invisible at abs=%d got val=%v typ=%v", abs, val, typ)
		}
	}
}

func TestTrimAfterPathChangeUsesLastImmutableAndFreeze(t *testing.T) {
	s := NewService()
	windowLen := 16
	capacity := 16

	// Immutable playback commits up to abs=5.
	for abs := 0; abs <= 5; abs++ {
		s.RecordCommitKind(0, abs, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, capacity)
	}
	// Speculative commits past the last immutable.
	for abs := 6; abs <= 9; abs++ {
		s.RecordCommitKind(0, abs, false, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)
	}

	// cutoffExclusive=4 would suggest keeping up to 3, but lastImmutable=5 should win.
	report := s.TrimAfterPathChange(0, 4, -1)
	if report.LastImmutable != 5 {
		t.Fatalf("expected lastImmutable=5 got %d", report.LastImmutable)
	}
	if report.MaxKeep != 5 {
		t.Fatalf("expected maxKeep=5 got %d", report.MaxKeep)
	}
	for abs := 6; abs <= 9; abs++ {
		if _, _, _, ok := s.CommittedKind(0, abs); ok {
			t.Fatalf("expected abs=%d trimmed after path change", abs)
		}
	}

	// When freeze limit exceeds last immutable, it should extend maxKeep.
	s.RecordCommitKind(1, 0, true, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)
	s.RecordCommitKind(1, 1, true, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)
	s.RecordCommitKind(1, 2, true, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)
	s.RecordCommitKind(1, 3, true, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)
	s.RecordCommitKind(1, 4, true, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)
	s.RecordCommitKind(1, 5, true, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)
	s.RecordCommitKind(1, 6, true, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)

	report = s.TrimAfterPathChange(1, 2, 5)
	if report.MaxKeep != 5 {
		t.Fatalf("expected maxKeep=5 got %d", report.MaxKeep)
	}
	if _, _, _, ok := s.CommittedKind(1, 6); ok {
		t.Fatalf("expected commit at abs=6 trimmed when freezeLimit=5")
	}
	if _, _, _, ok := s.CommittedKind(1, 5); !ok {
		t.Fatalf("expected commit at abs=5 preserved when freezeLimit=5")
	}
}

func TestTrimBeforeAfterPruneImmutables(t *testing.T) {
	s := NewService()
	windowLen := 8
	capacity := 8

	s.RecordCommitKind(0, 2, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, capacity)
	s.RecordCommitKind(0, 5, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, capacity)

	s.TrimBefore(0, 4)
	if _, _, _, ok := s.CommittedKind(0, 2); ok {
		t.Fatalf("expected abs=2 trimmed")
	}
	if _, _, _, ok := s.CommittedKind(0, 5); !ok {
		t.Fatalf("expected abs=5 preserved after TrimBefore")
	}
	if got := s.LastImmutableAbs(0); got != 5 {
		t.Fatalf("expected last immutable=5 got %d", got)
	}

	s.TrimAfter(0, 4)
	if _, _, _, ok := s.CommittedKind(0, 5); ok {
		t.Fatalf("expected abs=5 trimmed after TrimAfter")
	}
	if got := s.LastImmutableAbs(0); got != -1 {
		t.Fatalf("expected no immutable after TrimAfter got %d", got)
	}
}

func TestTrimAfterMutablePreservesImmutable(t *testing.T) {
	s := NewService()
	windowLen := 8
	capacity := 8

	s.RecordCommitKind(0, 4, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, capacity)
	s.RecordCommitKind(0, 5, true, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)
	s.RecordCommitKind(0, 6, true, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)

	s.TrimAfterMutable(0, 2)
	if _, _, _, ok := s.CommittedKind(0, 6); ok {
		t.Fatalf("expected seeded abs=6 trimmed")
	}
	if _, _, _, ok := s.CommittedKind(0, 5); ok {
		t.Fatalf("expected seeded abs=5 trimmed")
	}
	if _, _, kind, ok := s.CommittedKind(0, 4); !ok || kind != CommitKindPlayback {
		t.Fatalf("expected immutable abs=4 preserved; ok=%v kind=%v", ok, kind)
	}
}

func TestReplaceCommitDemotesPlaybackRemovesSidecar(t *testing.T) {
	s := NewService()
	windowLen := 8
	capacity := 8

	s.RecordCommitKind(0, 2, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, capacity)
	if got := s.LastImmutableAbs(0); got != 2 {
		t.Fatalf("expected last immutable abs=2 got %d", got)
	}

	if ok := s.ReplaceCommit(0, 2, true, model.NodeTypeRegular, CommitKindReleased); !ok {
		t.Fatalf("expected demotion replace to succeed")
	}
	val, typ, kind, ok := s.CommittedKind(0, 2)
	if !ok {
		t.Fatalf("expected commit present after demotion")
	}
	if !val || typ != model.NodeTypeRegular {
		t.Fatalf("unexpected demoted payload val=%v typ=%v", val, typ)
	}
	if kind != CommitKindReleased {
		t.Fatalf("expected demoted kind Released got %v", kind)
	}
	if got := s.LastImmutableAbs(0); got != -1 {
		t.Fatalf("expected last immutable abs cleared after demotion got %d", got)
	}
}

func TestCommittedRangeEmptyAndPopulated(t *testing.T) {
	s := NewService()
	if _, _, ok := s.CommittedRange(0); ok {
		t.Fatalf("expected empty committed range")
	}
	s.RecordCommitKind(0, 3, true, model.NodeTypeRegular, CommitKindSeeded, 8, 8)
	s.RecordCommitKind(0, 5, true, model.NodeTypeRegular, CommitKindSeeded, 8, 8)
	start, end, ok := s.CommittedRange(0)
	if !ok {
		t.Fatalf("expected committed range")
	}
	if start != 3 || end != 5 {
		t.Fatalf("unexpected committed range: %d..%d", start, end)
	}
}

func TestClearRowRemovesCommitsAndSidecar(t *testing.T) {
	s := NewService()
	s.RecordCommitKind(0, 0, true, model.NodeTypeRegular, CommitKindPlayback, 8, 8)
	s.RecordCommitKind(0, 1, true, model.NodeTypeRegular, CommitKindSeeded, 8, 8)

	s.ClearRow(0)
	if _, _, _, ok := s.CommittedKind(0, 0); ok {
		t.Fatalf("expected cleared row to have no commits")
	}
	if got := s.LastImmutableAbs(0); got != -1 {
		t.Fatalf("expected no immutable after ClearRow got %d", got)
	}
	if s.HasImmutable(0) {
		t.Fatalf("expected HasImmutable=false after ClearRow")
	}
}

func TestReplaceCommitPromotesSeededAddsSidecar(t *testing.T) {
	s := NewService()
	windowLen := 8
	capacity := 8

	s.RecordCommitKind(0, 5, true, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)
	if got := s.LastImmutableAbs(0); got != -1 {
		t.Fatalf("expected no immutable commits got %d", got)
	}

	if ok := s.ReplaceCommit(0, 5, true, model.NodeTypeRegular, CommitKindPlayback); !ok {
		t.Fatalf("expected promotion replace to succeed")
	}
	if got := s.LastImmutableAbs(0); got != 5 {
		t.Fatalf("expected last immutable abs=5 after promotion got %d", got)
	}
	_, _, kind, ok := s.CommittedKind(0, 5)
	if !ok {
		t.Fatalf("expected promoted commit present")
	}
	if kind != CommitKindPlayback {
		t.Fatalf("expected promoted kind Playback got %v", kind)
	}
}

func TestUpdateRowSegmentsPastMaskAndPresentFuture(t *testing.T) {
	s := NewService()
	windowLen := 4
	capacity := 4
	s.RecordCommitKind(0, 0, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, capacity)
	s.RecordCommitKind(0, 1, true, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)
	steps := []bool{false, true, false, true}

	var view SegmentsView
	s.UpdateRowSegments(0, 0, windowLen, capacity, steps, func(v SegmentsView) {
		view = v
	})
	if !view.Past[0] || !view.PastMask[0] {
		t.Fatalf("expected past[0] and mask set for playback commit")
	}
	if view.PastMask[1] {
		t.Fatalf("expected seeded commit to have mask=false")
	}
	if view.Present[1] != steps[1] || view.Future[3] != steps[3] {
		t.Fatalf("present/future should reflect steps; present=%v future=%v", view.Present, view.Future)
	}
}

func TestReplaceCommitReturnsFalseWhenAbsMissing(t *testing.T) {
	s := NewService()
	windowLen := 8
	capacity := 8

	s.RecordCommitKind(0, 0, true, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)

	if ok := s.ReplaceCommit(0, 7, true, model.NodeTypeRegular, CommitKindPlayback); ok {
		t.Fatalf("expected replace to fail when abs is not retained")
	}
	if _, _, _, ok := s.CommittedKind(0, 7); ok {
		t.Fatalf("expected missing commit at abs=7")
	}
}

func TestReplaceCommitDemotesPlaybackEvenIfRingMissing(t *testing.T) {
	s := NewService()
	windowLen := 8
	capacity := 8

	s.RecordCommitKind(0, 1, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, capacity)
	// Simulate the commit ring no longer retaining the entry while the immutable
	// sidecar still has it (e.g., due to a resize/trim in an earlier buggy build).
	s.rows[0].commits.clear()

	if ok := s.ReplaceCommit(0, 1, true, model.NodeTypeRegular, CommitKindReleased); !ok {
		t.Fatalf("expected demotion to succeed via sidecar path")
	}
	if _, _, _, ok := s.CommittedKind(0, 1); ok {
		t.Fatalf("expected no commit after demotion when ring is empty")
	}
	if got := s.LastImmutableAbs(0); got != -1 {
		t.Fatalf("expected sidecar cleared after demotion got %d", got)
	}
}

func TestSnapshotIsDeepCopy(t *testing.T) {
	s := NewService()
	windowLen := 8
	capacity := 8

	s.RecordCommitKind(0, 0, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, capacity)
	s.RecordCommitKind(0, 1, false, model.NodeTypeMute, CommitKindPlayback, windowLen, capacity)

	steps := make([]bool, windowLen)
	steps[0] = true
	steps[2] = true

	s.UpdateRowSegments(0, 0, windowLen, capacity, steps, func(SegmentsView) {})

	snap1 := s.Snapshot(0)
	if len(snap1.Past) != windowLen || len(snap1.Present) != windowLen {
		t.Fatalf("expected snapshot length=%d got past=%d present=%d", windowLen, len(snap1.Past), len(snap1.Present))
	}

	// Mutate the snapshot; service state must not change.
	snap1.Past[0] = false
	snap1.PastMask[0] = false
	snap1.PastTypes[0] = model.NodeTypeInvisible
	snap1.Present[0] = false
	snap1.Future[0] = false

	snap2 := s.Snapshot(0)
	if !snap2.Past[0] {
		t.Fatalf("expected service past[0] preserved after snapshot mutation")
	}
	if !snap2.PastMask[0] {
		t.Fatalf("expected service pastMask[0] preserved after snapshot mutation")
	}
	if snap2.PastTypes[0] != model.NodeTypeRegular {
		t.Fatalf("expected service pastTypes[0]=regular after snapshot mutation got %v", snap2.PastTypes[0])
	}
	if !snap2.Present[0] {
		t.Fatalf("expected service present[0] preserved after snapshot mutation")
	}
	if !snap2.Future[0] {
		t.Fatalf("expected service future[0] preserved after snapshot mutation")
	}
}

func TestServiceConcurrentAccessDoesNotPanic(t *testing.T) {
	s := NewService()
	windowLen := 16
	capacity := 32
	steps := make([]bool, windowLen)
	for i := range steps {
		steps[i] = i%3 == 0
	}

	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		for abs := 0; abs < 256; abs++ {
			s.RecordCommitKind(0, abs, abs%2 == 0, model.NodeTypeRegular, CommitKindSeeded, windowLen, capacity)
		}
	}()

	go func() {
		defer wg.Done()
		for abs := 0; abs < 256; abs++ {
			s.UpdateRowSegments(0, abs, windowLen, capacity, steps, func(SegmentsView) {})
		}
	}()

	go func() {
		defer wg.Done()
		for abs := 0; abs < 256; abs++ {
			_, _, _, _ = s.CommittedKind(0, abs)
			_ = s.Snapshot(0)
			_ = s.ReplaceCommit(0, abs, false, model.NodeTypeRegular, CommitKindReleased)
		}
	}()

	go func() {
		defer wg.Done()
		for abs := 0; abs < 256; abs++ {
			s.TrimAfterMutable(0, abs)
			s.LastCommitted(0)
			s.HasImmutable(0)
		}
	}()

	wg.Wait()
}
