package ui

import "testing"

func TestParityMismatchSnapshotAndClear(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	g.parityWatch = parityWatchLog
	g.parityReport(mismatchEntry{
		Row:      0,
		Abs:      3,
		Kind:     "test",
		Expected: true,
		Actual:   false,
		Source:   "unit",
	})

	snap1 := g.ParityMismatchSnapshot()
	if len(snap1) != 1 {
		t.Fatalf("expected 1 mismatch entry got %d", len(snap1))
	}
	if snap1[0].Abs != 3 || snap1[0].Row != 0 || snap1[0].Kind != "test" {
		t.Fatalf("unexpected mismatch entry: %+v", snap1[0])
	}

	// Ensure callers cannot mutate the ring via the snapshot.
	snap1[0].Abs = 999
	snap2 := g.ParityMismatchSnapshot()
	if len(snap2) != 1 {
		t.Fatalf("expected 1 mismatch entry after mutation got %d", len(snap2))
	}
	if snap2[0].Abs != 3 {
		t.Fatalf("expected snapshot to be a copy; abs=%d", snap2[0].Abs)
	}

	g.ClearParityMismatches()
	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected mismatch ring cleared got %d", got)
	}
}

func TestParityReportDoesNotRecordWhenWatchOffAndNonfatal(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	prevFatal := parityFatalEnabled.Load()
	parityFatalEnabled.Store(false)
	t.Cleanup(func() { parityFatalEnabled.Store(prevFatal) })

	g.parityWatch = parityWatchOff
	g.parityReport(mismatchEntry{
		Row:      0,
		Abs:      1,
		Kind:     "test_off",
		Expected: true,
		Actual:   false,
		Source:   "unit",
	})

	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected parity report ignored when watch off and nonfatal; got %d entries", got)
	}
}
