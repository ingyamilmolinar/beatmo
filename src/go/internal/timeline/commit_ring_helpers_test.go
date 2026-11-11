package timeline

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// fillRing pushes n monotonic appends so subsequent slot-shift logic
// has something to operate on.
func fillRing(t *testing.T, r *commitRing, base, n, capacity int) {
	t.Helper()
	r.ensureCapacity(capacity)
	for i := 0; i < n; i++ {
		r.append(base+i, i%2 == 0, model.NodeTypeRegular, CommitKindPlayback)
	}
}

// TestCommitRingClearLeavesCapacityIntact — clear() must zero size and
// re-set head/base but leave the underlying slice capacity reusable.
func TestCommitRingClearLeavesCapacityIntact(t *testing.T) {
	var r commitRing
	fillRing(t, &r, 0, 5, 8)
	capBefore := len(r.vals)
	r.clear()
	if r.size != 0 || r.head != 0 || r.base != 0 {
		t.Errorf("after clear: size=%d head=%d base=%d, want 0/0/0", r.size, r.head, r.base)
	}
	if len(r.vals) != capBefore {
		t.Errorf("clear shrunk underlying slice: %d -> %d", capBefore, len(r.vals))
	}
}

// TestCommitRingLastReportsCorrectAbs ensures last() returns base+size-1
// when populated and -1 when empty.
func TestCommitRingLastReportsCorrectAbs(t *testing.T) {
	var r commitRing
	if got := r.last(); got != -1 {
		t.Errorf("empty ring last()=%d, want -1", got)
	}
	fillRing(t, &r, 100, 4, 8)
	if got := r.last(); got != 103 {
		t.Errorf("populated ring last()=%d, want 103", got)
	}
}

// TestCommitRingTrimBeforeEdges covers the no-op (minAbs <= base) and
// full-clear (minAbs >= last) branches, plus a partial trim that exercises
// head advancement.
func TestCommitRingTrimBeforeEdges(t *testing.T) {
	var r commitRing
	fillRing(t, &r, 0, 6, 8)
	r.trimBefore(-5) // no-op
	if r.size != 6 || r.base != 0 {
		t.Errorf("trimBefore(-5) mutated state: size=%d base=%d", r.size, r.base)
	}
	r.trimBefore(3) // partial
	if r.base != 3 || r.size != 3 {
		t.Errorf("trimBefore(3): base=%d size=%d, want 3/3", r.base, r.size)
	}
	r.trimBefore(99) // full clear
	if r.size != 0 || r.base != 99 {
		t.Errorf("trimBefore(99): base=%d size=%d, want 99/0", r.base, r.size)
	}
}

// TestCommitRingTrimAfterEdges covers no-op, full clear (maxAbs < base),
// and partial trim from the high end.
func TestCommitRingTrimAfterEdges(t *testing.T) {
	var r commitRing
	fillRing(t, &r, 10, 6, 8)
	r.trimAfter(99) // no-op
	if r.size != 6 {
		t.Errorf("trimAfter(99) mutated size: %d", r.size)
	}
	r.trimAfter(13) // partial — keep [10..13]
	if start := r.base; start != 10 {
		t.Errorf("trimAfter(13): base=%d, want 10", start)
	}
	if r.size != 4 {
		t.Errorf("trimAfter(13): size=%d, want 4", r.size)
	}
	r.trimAfter(5) // below base — full clear
	if r.size != 0 {
		t.Errorf("trimAfter(5) below base: size=%d, want 0", r.size)
	}
}

// TestCommitRingTrimAfterMutableStopsAtImmutable — the helper must bail
// at the first Playback entry it sees from the high end.
func TestCommitRingTrimAfterMutableStopsAtImmutable(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)
	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(1, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(2, true, model.NodeTypeRegular, CommitKindSeeded)
	r.append(3, true, model.NodeTypeRegular, CommitKindSeeded)
	r.trimAfterMutable(1)
	// Two seeded entries at the top should have been popped.
	if r.size != 2 {
		t.Errorf("trimAfterMutable: size=%d, want 2 (immutables only)", r.size)
	}
	if r.last() != 1 {
		t.Errorf("trimAfterMutable: last=%d, want 1", r.last())
	}
}

// TestCommitRingReplaceInRange overwrites an existing entry and reports
// true; out-of-range writes report false without panicking.
func TestCommitRingReplaceInRange(t *testing.T) {
	var r commitRing
	fillRing(t, &r, 0, 4, 8)
	if !r.replace(2, false, model.NodeTypeMute, CommitKindSeeded) {
		t.Fatal("replace(in-range) returned false")
	}
	val, typ, kind, ok := r.value(2)
	if !ok || val != false || typ != model.NodeTypeMute || kind != CommitKindSeeded {
		t.Errorf("post-replace value: (%v,%v,%v,%v), want (false,Mute,Seeded,true)", val, typ, kind, ok)
	}
	if r.replace(99, true, model.NodeTypeRegular, CommitKindSeeded) {
		t.Error("replace(out-of-range) returned true")
	}
	if r.replace(-1, true, model.NodeTypeRegular, CommitKindSeeded) {
		t.Error("replace(-1) returned true")
	}
}

// TestCloneBoolSlice and TestCloneNodeTypeSlice cover the trivial
// defensive copies used by Snapshot. The contract is "nil -> nil, others
// -> independent copy".
func TestCloneBoolSlice(t *testing.T) {
	if got := cloneBoolSlice(nil); got != nil {
		t.Errorf("cloneBoolSlice(nil) = %v, want nil", got)
	}
	if got := cloneBoolSlice([]bool{}); got != nil {
		t.Errorf("cloneBoolSlice(empty) = %v, want nil", got)
	}
	src := []bool{true, false, true}
	dst := cloneBoolSlice(src)
	if len(dst) != len(src) {
		t.Fatalf("len mismatch: %d vs %d", len(dst), len(src))
	}
	dst[0] = !dst[0]
	if src[0] == dst[0] {
		t.Error("cloneBoolSlice produced a shared-storage copy")
	}
}

func TestCloneNodeTypeSlice(t *testing.T) {
	if got := cloneNodeTypeSlice(nil); got != nil {
		t.Errorf("cloneNodeTypeSlice(nil) = %v, want nil", got)
	}
	if got := cloneNodeTypeSlice([]model.NodeType{}); got != nil {
		t.Errorf("cloneNodeTypeSlice(empty) = %v, want nil", got)
	}
	src := []model.NodeType{model.NodeTypeRegular, model.NodeTypeMute}
	dst := cloneNodeTypeSlice(src)
	dst[0] = model.NodeTypeInvisible
	if src[0] == dst[0] {
		t.Error("cloneNodeTypeSlice produced a shared-storage copy")
	}
}

// TestArchiveAppendLookupTrim drives the rowArchive in isolation: a
// monotonic append fast path, an out-of-order insert (sorted insert),
// trimBefore/trimAfter slicing, and clear.
func TestArchiveAppendLookupTrim(t *testing.T) {
	var a rowArchive
	for i := 0; i < 8; i++ {
		a.append(i, i%2 == 0, model.NodeTypeRegular, CommitKindPlayback, 0)
	}
	if a.lenForTest() != 8 {
		t.Fatalf("after 8 appends, len=%d", a.lenForTest())
	}
	// Out-of-order insert (sorted path).
	a.append(3, true, model.NodeTypeMute, CommitKindImport, 0)
	if got := a.lenForTest(); got != 8 {
		t.Errorf("after replacement append: len=%d, want 8", got)
	}
	val, typ, kind, ok := a.lookup(3)
	if !ok {
		t.Fatal("lookup(3) ok=false")
	}
	// Replace ONLY succeeds when the existing entry isn't already an
	// immutable kind. abs=3 was Playback, so the Import insert should be
	// rejected.
	if kind == CommitKindImport {
		t.Errorf("immutable Playback at abs=3 was overwritten with Import: val=%v typ=%v", val, typ)
	}

	a.trimBefore(2)
	if a.lenForTest() != 6 {
		t.Errorf("after trimBefore(2): len=%d, want 6", a.lenForTest())
	}
	if _, _, _, ok := a.lookup(0); ok {
		t.Error("lookup(0) ok=true after trimBefore(2)")
	}

	a.trimAfter(5)
	if a.lenForTest() != 4 {
		t.Errorf("after trimAfter(5): len=%d, want 4", a.lenForTest())
	}
	if _, _, _, ok := a.lookup(6); ok {
		t.Error("lookup(6) ok=true after trimAfter(5)")
	}

	a.clear()
	if a.lenForTest() != 0 {
		t.Errorf("after clear: len=%d, want 0", a.lenForTest())
	}
}

// TestArchiveEvictChunkOnOverflow proves that maxEntries triggers a
// drop-oldest in archiveEvictChunk-sized batches; the post-condition is
// len(entries) <= maxEntries (within one eviction batch).
func TestArchiveEvictChunkOnOverflow(t *testing.T) {
	var a rowArchive
	maxEntries := 4 // small, to exercise the eviction immediately.
	for i := 0; i < archiveEvictChunk+maxEntries+10; i++ {
		a.append(i, false, model.NodeTypeRegular, CommitKindPlayback, maxEntries)
	}
	if got := a.lenForTest(); got > maxEntries {
		t.Errorf("post-overflow len=%d > maxEntries=%d", got, maxEntries)
	}
}

// TestArchiveLookupEmptyAndNotFound — nil archive must be safe; absent
// abs must report ok=false without panicking.
func TestArchiveLookupEmptyAndNotFound(t *testing.T) {
	var nilArch *rowArchive
	if _, _, _, ok := nilArch.lookup(0); ok {
		t.Error("nil archive lookup ok=true")
	}
	var a rowArchive
	if _, _, _, ok := a.lookup(0); ok {
		t.Error("empty archive lookup ok=true")
	}
	a.append(5, true, model.NodeTypeRegular, CommitKindPlayback, 0)
	if _, _, _, ok := a.lookup(7); ok {
		t.Error("lookup(7) ok=true on archive containing only abs=5")
	}
}

// TestArchiveNilSafetyOnTrimsAndClear — every helper is nil-safe so
// callers don't have to guard.
func TestArchiveNilSafetyOnTrimsAndClear(t *testing.T) {
	var nilArch *rowArchive
	nilArch.trimBefore(0)
	nilArch.trimAfter(0)
	nilArch.clear()
	if nilArch.lenForTest() != 0 {
		t.Errorf("nil archive lenForTest=%d, want 0", nilArch.lenForTest())
	}
}
