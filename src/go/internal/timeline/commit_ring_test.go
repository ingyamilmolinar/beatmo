package timeline

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestCommitRingLastEmpty(t *testing.T) {
	var r commitRing
	if got := r.last(); got != -1 {
		t.Fatalf("expected last() == -1 for empty ring, got %d", got)
	}
}

func TestCommitRingLastAfterAppend(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	r.append(5, true, model.NodeTypeRegular, CommitKindPlayback)
	if got := r.last(); got != 5 {
		t.Fatalf("expected last() == 5 after first append, got %d", got)
	}

	r.append(6, false, model.NodeTypeRegular, CommitKindPlayback)
	if got := r.last(); got != 6 {
		t.Fatalf("expected last() == 6 after second append, got %d", got)
	}
}

func TestCommitRingAppendAndValue(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback)

	val, typ, kind, ok := r.value(0)
	if !ok {
		t.Fatal("expected value(0) to be present")
	}
	if !val {
		t.Error("expected val == true")
	}
	if typ != model.NodeTypeRegular {
		t.Errorf("expected NodeTypeRegular, got %v", typ)
	}
	if kind != CommitKindPlayback {
		t.Errorf("expected CommitKindPlayback, got %v", kind)
	}

	_, _, _, ok = r.value(1)
	if ok {
		t.Error("expected value(1) to be absent")
	}
}

func TestCommitRingTrimBefore(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	for i := 0; i < 4; i++ {
		r.append(i, true, model.NodeTypeRegular, CommitKindPlayback)
	}

	r.trimBefore(2)

	for _, abs := range []int{0, 1} {
		if _, _, _, ok := r.value(abs); ok {
			t.Errorf("expected value(%d) gone after trimBefore(2)", abs)
		}
	}
	for _, abs := range []int{2, 3} {
		if _, _, _, ok := r.value(abs); !ok {
			t.Errorf("expected value(%d) still present after trimBefore(2)", abs)
		}
	}
}

func TestCommitRingTrimAfter(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	for i := 0; i < 4; i++ {
		r.append(i, true, model.NodeTypeRegular, CommitKindPlayback)
	}

	r.trimAfter(1)

	for _, abs := range []int{0, 1} {
		if _, _, _, ok := r.value(abs); !ok {
			t.Errorf("expected value(%d) still present after trimAfter(1)", abs)
		}
	}
	for _, abs := range []int{2, 3} {
		if _, _, _, ok := r.value(abs); ok {
			t.Errorf("expected value(%d) gone after trimAfter(1)", abs)
		}
	}
}

func TestCommitRingTrimAfterMutable(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(1, true, model.NodeTypeRegular, CommitKindSeeded)
	r.append(2, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(3, true, model.NodeTypeRegular, CommitKindSeeded)
	r.append(4, true, model.NodeTypeRegular, CommitKindReleased)

	r.trimAfterMutable(1)

	// abs=4 (Released/mutable) and abs=3 (Seeded/mutable) should be removed.
	// Removal stops at abs=2 (Playback/immutable).
	if _, _, _, ok := r.value(4); ok {
		t.Error("expected abs=4 (Released) removed")
	}
	if _, _, _, ok := r.value(3); ok {
		t.Error("expected abs=3 (Seeded) removed")
	}
	// abs=2 is immutable so trimming stops; it should still be present.
	if _, _, _, ok := r.value(2); !ok {
		t.Error("expected abs=2 (Playback) still present")
	}
	// abs=0 and abs=1 should also remain.
	if _, _, _, ok := r.value(0); !ok {
		t.Error("expected abs=0 still present")
	}
	if _, _, _, ok := r.value(1); !ok {
		t.Error("expected abs=1 still present")
	}
}

func TestCommitRingCapacityGrowth(t *testing.T) {
	var r commitRing
	r.ensureCapacity(2)

	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(1, false, model.NodeTypeSilent, CommitKindSeeded)
	// Third append exceeds initial capacity; ring should auto-grow.
	r.append(2, true, model.NodeTypeMute, CommitKindImport)

	for _, abs := range []int{0, 1, 2} {
		if _, _, _, ok := r.value(abs); !ok {
			t.Errorf("expected value(%d) present after capacity growth", abs)
		}
	}

	val, typ, kind, _ := r.value(0)
	if !val || typ != model.NodeTypeRegular || kind != CommitKindPlayback {
		t.Errorf("abs=0 mismatch: val=%v typ=%v kind=%v", val, typ, kind)
	}
	val, typ, kind, _ = r.value(1)
	if val || typ != model.NodeTypeSilent || kind != CommitKindSeeded {
		t.Errorf("abs=1 mismatch: val=%v typ=%v kind=%v", val, typ, kind)
	}
	val, typ, kind, _ = r.value(2)
	if !val || typ != model.NodeTypeMute || kind != CommitKindImport {
		t.Errorf("abs=2 mismatch: val=%v typ=%v kind=%v", val, typ, kind)
	}
}

func TestCommitRingGapPadding(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback)
	// Skip abs=1 and abs=2; append at abs=3 creates a gap.
	r.append(3, true, model.NodeTypeRegular, CommitKindPlayback)

	for _, abs := range []int{1, 2} {
		val, typ, kind, ok := r.value(abs)
		if !ok {
			t.Fatalf("expected gap-padded entry at abs=%d", abs)
		}
		if val {
			t.Errorf("abs=%d: expected val=false for gap pad", abs)
		}
		if typ != model.NodeTypeInvisible {
			t.Errorf("abs=%d: expected NodeTypeInvisible for gap pad, got %v", abs, typ)
		}
		if kind != CommitKindGapPad {
			t.Errorf("abs=%d: expected CommitKindGapPad, got %v", abs, kind)
		}
	}
}

func TestCommitRingPlaybackProtection(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback)
	// Attempt to overwrite with a mutable kind.
	r.append(0, false, model.NodeTypeSilent, CommitKindSeeded)

	val, typ, kind, ok := r.value(0)
	if !ok {
		t.Fatal("expected value(0) present")
	}
	if !val {
		t.Error("expected original val=true preserved")
	}
	if typ != model.NodeTypeRegular {
		t.Error("expected original NodeTypeRegular preserved")
	}
	if kind != CommitKindPlayback {
		t.Error("expected original CommitKindPlayback preserved")
	}
}

func TestCommitRingReplace(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	r.append(0, false, model.NodeTypeSilent, CommitKindSeeded)

	ok := r.replace(0, true, model.NodeTypeRegular, CommitKindReleased)
	if !ok {
		t.Fatal("expected replace to succeed on existing entry")
	}

	val, typ, kind, present := r.value(0)
	if !present {
		t.Fatal("expected value(0) present after replace")
	}
	if !val {
		t.Error("expected val=true after replace")
	}
	if typ != model.NodeTypeRegular {
		t.Errorf("expected NodeTypeRegular after replace, got %v", typ)
	}
	if kind != CommitKindReleased {
		t.Errorf("expected CommitKindReleased after replace, got %v", kind)
	}

	// Replace on non-existent abs should return false.
	if r.replace(99, true, model.NodeTypeRegular, CommitKindPlayback) {
		t.Error("expected replace on non-existent abs to return false")
	}
}

func TestCommitRingClear(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(1, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(2, true, model.NodeTypeRegular, CommitKindPlayback)

	r.clear()

	if got := r.last(); got != -1 {
		t.Fatalf("expected last() == -1 after clear, got %d", got)
	}
	for _, abs := range []int{0, 1, 2} {
		if _, _, _, ok := r.value(abs); ok {
			t.Errorf("expected value(%d) absent after clear", abs)
		}
	}
}

func TestRingWrapAroundLargeVolume(t *testing.T) {
	var r commitRing
	r.ensureCapacity(4) // start small to force multiple growths

	const count = 120
	for i := 0; i < count; i++ {
		val := i%2 == 0
		r.append(i, val, model.NodeTypeRegular, CommitKindPlayback)
	}

	if got := r.last(); got != count-1 {
		t.Fatalf("expected last() == %d, got %d", count-1, got)
	}

	// Verify every entry is still readable with correct values.
	for i := 0; i < count; i++ {
		val, typ, kind, ok := r.value(i)
		if !ok {
			t.Fatalf("expected value(%d) present after %d appends", i, count)
		}
		expectedVal := i%2 == 0
		if val != expectedVal {
			t.Errorf("abs=%d: expected val=%v, got %v", i, expectedVal, val)
		}
		if typ != model.NodeTypeRegular {
			t.Errorf("abs=%d: expected NodeTypeRegular, got %v", i, typ)
		}
		if kind != CommitKindPlayback {
			t.Errorf("abs=%d: expected CommitKindPlayback, got %v", i, kind)
		}
	}

	// Reading beyond the last entry should fail.
	if _, _, _, ok := r.value(count); ok {
		t.Errorf("expected value(%d) absent (beyond last)", count)
	}
}

func TestImmutableCommitPreservation(t *testing.T) {
	svc := NewService()
	row := 0
	abs := 5
	windowLen := 16
	capacity := 16

	// Record a playback (immutable) commit.
	svc.RecordCommitKind(row, abs, true, model.NodeTypeRegular, CommitKindPlayback, windowLen, capacity)

	// Verify it was stored.
	val, typ, kind, ok := svc.CommittedKind(row, abs)
	if !ok {
		t.Fatal("expected playback commit to be present")
	}
	if !val || typ != model.NodeTypeRegular || kind != CommitKindPlayback {
		t.Fatalf("unexpected initial commit: val=%v typ=%v kind=%v", val, typ, kind)
	}

	// Attempt to overwrite with a seeded (mutable) commit at the same position.
	svc.RecordCommitKind(row, abs, false, model.NodeTypeSilent, CommitKindSeeded, windowLen, capacity)

	// The immutable playback commit must be preserved.
	val, typ, kind, ok = svc.CommittedKind(row, abs)
	if !ok {
		t.Fatal("expected commit still present after overwrite attempt")
	}
	if !val {
		t.Error("expected original val=true preserved, got false")
	}
	if typ != model.NodeTypeRegular {
		t.Errorf("expected original NodeTypeRegular preserved, got %v", typ)
	}
	if kind != CommitKindPlayback {
		t.Errorf("expected CommitKindPlayback preserved, got %v", kind)
	}
}

func TestRingTrimBeforePreservesRecent(t *testing.T) {
	var r commitRing
	r.ensureCapacity(16)

	// Append entries at indices 0 through 9.
	for i := 0; i < 10; i++ {
		r.append(i, true, model.NodeTypeRegular, CommitKindSeeded)
	}

	// Trim everything before index 5.
	r.trimBefore(5)

	// Indices 0-4 should be gone.
	for abs := 0; abs < 5; abs++ {
		if _, _, _, ok := r.value(abs); ok {
			t.Errorf("expected value(%d) gone after trimBefore(5)", abs)
		}
	}

	// Indices 5-9 should remain with correct values.
	for abs := 5; abs < 10; abs++ {
		val, typ, kind, ok := r.value(abs)
		if !ok {
			t.Errorf("expected value(%d) still present after trimBefore(5)", abs)
			continue
		}
		if !val {
			t.Errorf("abs=%d: expected val=true", abs)
		}
		if typ != model.NodeTypeRegular {
			t.Errorf("abs=%d: expected NodeTypeRegular, got %v", abs, typ)
		}
		if kind != CommitKindSeeded {
			t.Errorf("abs=%d: expected CommitKindSeeded, got %v", abs, kind)
		}
	}

	// last() should still be 9.
	if got := r.last(); got != 9 {
		t.Fatalf("expected last() == 9 after trimBefore(5), got %d", got)
	}
}

func TestRingTrimAfterMutableStopsAtImmutable(t *testing.T) {
	var r commitRing
	r.ensureCapacity(16)

	// Build a sequence: immutable, mutable, immutable, mutable, mutable, mutable
	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback) // immutable
	r.append(1, true, model.NodeTypeRegular, CommitKindSeeded)   // mutable
	r.append(2, true, model.NodeTypeRegular, CommitKindImport)   // immutable
	r.append(3, true, model.NodeTypeRegular, CommitKindGapPad)   // mutable
	r.append(4, true, model.NodeTypeRegular, CommitKindReleased) // mutable
	r.append(5, true, model.NodeTypeRegular, CommitKindSeeded)   // mutable

	// trimAfterMutable should remove trailing mutable entries from the end
	// and stop when it hits an immutable entry.
	r.trimAfterMutable(0)

	// abs=5, 4, 3 are mutable and should be removed.
	for _, abs := range []int{5, 4, 3} {
		if _, _, _, ok := r.value(abs); ok {
			t.Errorf("expected abs=%d (mutable) to be removed", abs)
		}
	}

	// abs=2 is immutable (Import), trimming should have stopped here.
	if _, _, _, ok := r.value(2); !ok {
		t.Error("expected abs=2 (Import/immutable) still present")
	}

	// abs=0 and abs=1 should also remain (below the immutable stop point).
	if _, _, _, ok := r.value(0); !ok {
		t.Error("expected abs=0 (Playback) still present")
	}
	if _, _, _, ok := r.value(1); !ok {
		t.Error("expected abs=1 (Seeded) still present")
	}
}

func TestCommitRingEnsureCapacityShrink(t *testing.T) {
	var r commitRing
	r.ensureCapacity(16)

	// Fill 10 entries.
	for i := 0; i < 10; i++ {
		r.append(i, true, model.NodeTypeRegular, CommitKindPlayback)
	}

	// ensureCapacity with a value smaller than current capacity is a no-op
	// (the guard `capacity <= len(r.vals)` returns early).
	r.ensureCapacity(4)

	// All 10 entries should still be accessible — capacity didn't shrink.
	for i := 0; i < 10; i++ {
		if _, _, _, ok := r.value(i); !ok {
			t.Errorf("expected value(%d) still present after ensureCapacity(4)", i)
		}
	}
	if got := r.last(); got != 9 {
		t.Fatalf("expected last() == 9 after ensureCapacity(4), got %d", got)
	}
}

func TestCommitRingEnsureCapacityZero(t *testing.T) {
	var r commitRing
	// ensureCapacity(0) on a zero-value ring should be a no-op (0 <= 0).
	r.ensureCapacity(0)
	if len(r.vals) != 0 {
		t.Fatalf("expected vals to remain nil/empty, got len=%d", len(r.vals))
	}
	if r.size != 0 || r.base != 0 || r.head != 0 {
		t.Fatalf("unexpected state: size=%d base=%d head=%d", r.size, r.base, r.head)
	}

	// After calling ensureCapacity(0), appending should be a no-op because
	// len(r.vals) == 0.
	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback)
	if r.size != 0 {
		t.Fatalf("expected size=0 after append on zero-capacity ring, got %d", r.size)
	}
}

func TestCommitRingAppendGapExceedsCap(t *testing.T) {
	var r commitRing
	r.ensureCapacity(4) // start with cap=4

	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback)
	// Jump to abs=100 — gap of 99 entries forces multiple auto-growths.
	r.append(100, true, model.NodeTypeMute, CommitKindSeeded)

	// Verify the original entry survived.
	val, typ, kind, ok := r.value(0)
	if !ok {
		t.Fatal("expected value(0) present after large gap")
	}
	if !val || typ != model.NodeTypeRegular || kind != CommitKindPlayback {
		t.Fatalf("abs=0 mismatch: val=%v typ=%v kind=%v", val, typ, kind)
	}

	// Verify the distant entry.
	val, typ, kind, ok = r.value(100)
	if !ok {
		t.Fatal("expected value(100) present after large gap")
	}
	if !val || typ != model.NodeTypeMute || kind != CommitKindSeeded {
		t.Fatalf("abs=100 mismatch: val=%v typ=%v kind=%v", val, typ, kind)
	}

	// Gap entries 1..99 should all be GapPad.
	for abs := 1; abs < 100; abs++ {
		v, tp, k, ok := r.value(abs)
		if !ok {
			t.Fatalf("expected gap-padded entry at abs=%d", abs)
		}
		if v {
			t.Errorf("abs=%d: expected val=false for gap pad", abs)
		}
		if tp != model.NodeTypeInvisible {
			t.Errorf("abs=%d: expected NodeTypeInvisible, got %v", abs, tp)
		}
		if k != CommitKindGapPad {
			t.Errorf("abs=%d: expected CommitKindGapPad, got %v", abs, k)
		}
	}

	if got := r.last(); got != 100 {
		t.Fatalf("expected last() == 100, got %d", got)
	}
}

func TestCommitRingAppendBeforeBase(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	// Establish ring with base=5.
	r.append(5, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(6, true, model.NodeTypeRegular, CommitKindPlayback)

	// Append at abs < base — the in-range check `abs >= r.base` fails, so it's
	// a silent no-op (the entry is at abs <= last but < base).
	r.append(3, false, model.NodeTypeSilent, CommitKindSeeded)

	// abs=3 should not exist — it was before the base.
	if _, _, _, ok := r.value(3); ok {
		t.Fatal("expected value(3) absent — append before base should be no-op")
	}

	// Original entries should remain unchanged.
	val, typ, kind, ok := r.value(5)
	if !ok || !val || typ != model.NodeTypeRegular || kind != CommitKindPlayback {
		t.Fatalf("abs=5 corrupted after append-before-base: val=%v typ=%v kind=%v ok=%v", val, typ, kind, ok)
	}
}

func TestCommitRingTrimBeforeNoOp(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	r.append(5, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(6, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(7, true, model.NodeTypeRegular, CommitKindPlayback)

	// trimBefore with minAbs <= base should be a no-op.
	r.trimBefore(5) // exactly at base
	if r.size != 3 || r.base != 5 {
		t.Fatalf("expected no change: size=%d base=%d", r.size, r.base)
	}

	r.trimBefore(3) // below base
	if r.size != 3 || r.base != 5 {
		t.Fatalf("expected no change: size=%d base=%d", r.size, r.base)
	}

	// All entries should remain.
	for _, abs := range []int{5, 6, 7} {
		if _, _, _, ok := r.value(abs); !ok {
			t.Errorf("expected value(%d) still present", abs)
		}
	}
}

func TestCommitRingTrimBeforeBeyondAll(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(1, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(2, true, model.NodeTypeRegular, CommitKindPlayback)

	// trimBefore with minAbs >= base+size clears everything.
	r.trimBefore(10)

	if r.size != 0 {
		t.Fatalf("expected size=0 after trimBefore(10), got %d", r.size)
	}
	if r.base != 10 {
		t.Fatalf("expected base=10 after trimBefore(10), got %d", r.base)
	}
	for abs := 0; abs <= 2; abs++ {
		if _, _, _, ok := r.value(abs); ok {
			t.Errorf("expected value(%d) absent after trimBefore(10)", abs)
		}
	}
}

func TestCommitRingTrimAfterBelowBase(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	r.append(5, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(6, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(7, true, model.NodeTypeRegular, CommitKindPlayback)

	// trimAfter with maxAbs < base clears all entries.
	r.trimAfter(2)

	if r.size != 0 {
		t.Fatalf("expected size=0 after trimAfter(2) when base=5, got %d", r.size)
	}
	for _, abs := range []int{5, 6, 7} {
		if _, _, _, ok := r.value(abs); ok {
			t.Errorf("expected value(%d) absent after trimAfter(2)", abs)
		}
	}
}

func TestCommitRingValueEmptyRing(t *testing.T) {
	var r commitRing // zero value, no ensureCapacity called

	val, typ, kind, ok := r.value(0)
	if ok {
		t.Fatal("expected value() to return false on zero-value ring")
	}
	if val {
		t.Error("expected val=false")
	}
	if typ != model.NodeTypeInvisible {
		t.Errorf("expected NodeTypeInvisible, got %v", typ)
	}
	if kind != CommitKindPlayback {
		t.Errorf("expected CommitKindPlayback default, got %v", kind)
	}

	// Also test with arbitrary absolute index.
	_, _, _, ok = r.value(42)
	if ok {
		t.Error("expected value(42) to return false on zero-value ring")
	}
}

func TestCommitRingReplaceEmptyKinds(t *testing.T) {
	var r commitRing
	// Build a ring with size=0 — replace should return false.
	r.ensureCapacity(8)

	ok := r.replace(0, true, model.NodeTypeRegular, CommitKindPlayback)
	if ok {
		t.Fatal("expected replace to return false on empty ring")
	}

	// Append one entry then try replace at an out-of-range abs.
	r.append(5, true, model.NodeTypeRegular, CommitKindSeeded)
	ok = r.replace(99, true, model.NodeTypeRegular, CommitKindPlayback)
	if ok {
		t.Fatal("expected replace to return false for out-of-range abs")
	}

	// Replace below base should also return false.
	ok = r.replace(0, true, model.NodeTypeRegular, CommitKindPlayback)
	if ok {
		t.Fatal("expected replace to return false for abs below base")
	}
}

func TestCommitRingEnsureCapacityCorruptedKinds(t *testing.T) {
	var r commitRing
	r.ensureCapacity(8)

	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback)
	r.append(1, false, model.NodeTypeSilent, CommitKindSeeded)

	// Simulate a corrupted state: kinds/vals length mismatch.
	// Truncate kinds to trigger the self-healing rebuild at the top of
	// ensureCapacity.
	r.kinds = r.kinds[:1] // len(kinds)=1 != len(vals)=8

	// This call should trigger the self-healing: `r.kinds = make([]CommitKind, len(r.vals))`
	// then return early because capacity(16) > len(r.vals)=8 triggers the growth path.
	r.ensureCapacity(16)

	// After rebuilding, kinds should be properly sized.
	if len(r.kinds) != len(r.vals) {
		t.Fatalf("expected kinds len=%d to match vals len=%d after heal", len(r.kinds), len(r.vals))
	}

	// The ring should still function correctly — the first entry's data
	// should be preserved (it was copied during regrow).
	val, typ, _, ok := r.value(0)
	if !ok {
		t.Fatal("expected value(0) present after self-healing rebuild")
	}
	if !val || typ != model.NodeTypeRegular {
		t.Fatalf("abs=0 mismatch after rebuild: val=%v typ=%v", val, typ)
	}
}

func TestRingGapPaddingLargeGap(t *testing.T) {
	var r commitRing
	r.ensureCapacity(16)

	// Append at index 0.
	r.append(0, true, model.NodeTypeRegular, CommitKindPlayback)

	// Append at index 10, creating a gap of 9 indices (1-9).
	r.append(10, true, model.NodeTypeMute, CommitKindSeeded)

	// Verify the original entry at 0 is intact.
	val, typ, kind, ok := r.value(0)
	if !ok {
		t.Fatal("expected value(0) present")
	}
	if !val || typ != model.NodeTypeRegular || kind != CommitKindPlayback {
		t.Fatalf("abs=0 mismatch: val=%v typ=%v kind=%v", val, typ, kind)
	}

	// Verify the entry at 10 is correct.
	val, typ, kind, ok = r.value(10)
	if !ok {
		t.Fatal("expected value(10) present")
	}
	if !val || typ != model.NodeTypeMute || kind != CommitKindSeeded {
		t.Fatalf("abs=10 mismatch: val=%v typ=%v kind=%v", val, typ, kind)
	}

	// Verify all gap indices 1-9 are filled with CommitKindGapPad.
	for abs := 1; abs <= 9; abs++ {
		val, typ, kind, ok := r.value(abs)
		if !ok {
			t.Fatalf("expected gap-padded entry at abs=%d", abs)
		}
		if val {
			t.Errorf("abs=%d: expected val=false for gap pad, got true", abs)
		}
		if typ != model.NodeTypeInvisible {
			t.Errorf("abs=%d: expected NodeTypeInvisible for gap pad, got %v", abs, typ)
		}
		if kind != CommitKindGapPad {
			t.Errorf("abs=%d: expected CommitKindGapPad, got %v", abs, kind)
		}
	}

	// last() should be 10.
	if got := r.last(); got != 10 {
		t.Fatalf("expected last() == 10, got %d", got)
	}
}
