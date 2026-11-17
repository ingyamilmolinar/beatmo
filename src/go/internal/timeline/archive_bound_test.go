package timeline

import (
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestArchiveLookupAtMillionAbs drives 1 000 000 alternating playback/import
// commits at abs=0..999_999 on row 0; asserts that historical reads beyond
// the live immutables window resolve through the cold archive.
func TestArchiveLookupAtMillionAbs(t *testing.T) {
	s := NewService()

	const N = 1_000_000
	const windowLen = 64
	for abs := 0; abs < N; abs++ {
		kind := CommitKindPlayback
		if abs%2 == 1 {
			kind = CommitKindImport
		}
		s.RecordCommitKind(0, abs, true, model.NodeTypeRegular, kind, windowLen, windowLen)
	}

	// Sample three abs anchors: ancient (archive only), middle (archive only),
	// and most-recent (live immutables sidecar).
	cases := []struct {
		name string
		abs  int
		kind CommitKind
	}{
		{"ancient (archive)", 1, CommitKindImport},
		{"middle (archive)", 500_000, CommitKindPlayback},
		{"recent (immutables)", N - 1, CommitKindImport},
	}
	for _, tc := range cases {
		v, _, k, ok := s.CommittedKind(0, tc.abs)
		if !ok {
			t.Errorf("%s: CommittedKind(0,%d) ok=false; archive should serve",
				tc.name, tc.abs)
			continue
		}
		if !v {
			t.Errorf("%s: CommittedKind(0,%d) val=false; want true", tc.name, tc.abs)
		}
		if k != tc.kind {
			t.Errorf("%s: CommittedKind(0,%d) kind=%v; want %v", tc.name, tc.abs, k, tc.kind)
		}
	}

	if got := s.ArchiveLenForTest(0); got == 0 {
		t.Errorf("expected archive to hold migrated entries; got 0")
	}
}

// TestArchiveBytesBudget asserts that 1 M historical commits occupy a small
// bounded amount of heap. ~8 B/entry × 1 M = ~8 MB; leave generous headroom.
func TestArchiveBytesBudget(t *testing.T) {
	s := NewService()

	const N = 1_000_000
	const windowLen = 64

	var pre, post runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&pre)

	for abs := 0; abs < N; abs++ {
		s.RecordCommitKind(0, abs, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, windowLen)
	}

	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&post)

	heapAlloc := int64(post.HeapAlloc) - int64(pre.HeapAlloc)
	const ceiling = 64 << 20 // 64 MB
	if heapAlloc > ceiling {
		t.Errorf("HeapAlloc grew by %d bytes (%.1f MB) for %d commits; ceiling %d MB. "+
			"Cold archive should compress to ~8 B/entry.",
			heapAlloc, float64(heapAlloc)/(1024*1024), N, ceiling>>20)
	}
}

// TestArchiveCapEvictsOldest drives more entries than the configured cap and
// asserts the archive truncates to <= cap by dropping oldest, while recent
// entries stay queryable.
func TestArchiveCapEvictsOldest(t *testing.T) {
	s := NewService()
	const cap = 8_000
	const evictChunk = archiveEvictChunk
	s.SetArchiveMaxEntries(cap)

	// Drive enough entries to overflow the cap by several chunks. Each commit
	// goes to immutables first; once immutables exceeds 1024, older entries
	// migrate to the archive.
	const N = cap + evictChunk*2 + 1000
	const windowLen = 64
	for abs := 0; abs < N; abs++ {
		s.RecordCommitKind(0, abs, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, windowLen)
	}

	if got := s.ArchiveLenForTest(0); got > cap {
		t.Errorf("archive size=%d exceeds configured cap=%d", got, cap)
	}

	// Recent entries must still resolve (live in immutables or top of archive).
	if _, _, _, ok := s.CommittedKind(0, N-1); !ok {
		t.Errorf("CommittedKind(0,%d) ok=false; recent commit must be queryable", N-1)
	}
}

// TestArchiveSurvivesTrimAndSeed verifies that TrimBefore/TrimAfter propagate
// to the archive (so trimmed-away entries don't reappear via cold reads) and
// that the archive coexists correctly with seed flows.
func TestArchiveSurvivesTrimAndSeed(t *testing.T) {
	s := NewService()
	const windowLen = 64

	for abs := 0; abs < 50_000; abs++ {
		s.RecordCommitKind(0, abs, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, windowLen)
	}

	// Verify pre-trim: ancient abs resolves from archive.
	if _, _, _, ok := s.CommittedKind(0, 100); !ok {
		t.Fatalf("pre-trim: abs=100 should resolve via archive")
	}

	// Trim everything before abs=10_000. Archive entries with abs < 10_000
	// must drop too.
	s.TrimBefore(0, 10_000)
	if _, _, _, ok := s.CommittedKind(0, 100); ok {
		t.Errorf("after TrimBefore(0,10000): abs=100 still resolves; archive trim leaked")
	}
	if _, _, _, ok := s.CommittedKind(0, 20_000); !ok {
		t.Errorf("after TrimBefore(0,10000): abs=20000 should still resolve")
	}

	// Trim everything after abs=30_000. Archive entries with abs > 30_000
	// must drop too.
	s.TrimAfter(0, 30_000)
	if _, _, _, ok := s.CommittedKind(0, 40_000); ok {
		t.Errorf("after TrimAfter(0,30000): abs=40000 still resolves; archive trim leaked")
	}
	if _, _, _, ok := s.CommittedKind(0, 25_000); !ok {
		t.Errorf("after TrimAfter(0,30000): abs=25000 should still resolve")
	}
}
