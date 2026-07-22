package eventlogger

import (
	"testing"
	"time"
)

func TestRingHistoryRetainsNewestFirstWithinCap(t *testing.T) {
	h := NewRingHistory(3)
	for i := 1; i <= 5; i++ {
		h.Record(LogRecord{Seq: int64(i), At: time.Unix(int64(i), 0), Msg: "m"})
	}
	snap := h.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("len=%d, want 3 (cap enforced)", len(snap))
	}
	// Newest-first: seq 5,4,3 (1,2 evicted).
	wantSeq := []int64{5, 4, 3}
	for i, w := range wantSeq {
		if snap[i].Seq != w {
			t.Fatalf("snap[%d].Seq=%d, want %d", i, snap[i].Seq, w)
		}
	}
}

func TestRingHistoryClear(t *testing.T) {
	h := NewRingHistory(5)
	for i := 1; i <= 4; i++ {
		h.Record(LogRecord{Seq: int64(i), At: time.Unix(int64(i), 0), Msg: "m"})
	}
	h.Clear()
	if snap := h.Snapshot(); len(snap) != 0 {
		t.Fatalf("after Clear, Snapshot len=%d want 0", len(snap))
	}
	if got := h.LatestSeq(); got != 0 {
		t.Fatalf("after Clear, LatestSeq=%d want 0", got)
	}
	// Cleared history must still accept new records.
	h.Record(LogRecord{Seq: 9, Msg: "new"})
	if snap := h.Snapshot(); len(snap) != 1 || snap[0].Seq != 9 {
		t.Fatalf("record after Clear failed: %+v", snap)
	}
}

func TestSnapshotIsCopy(t *testing.T) {
	h := NewRingHistory(2)
	h.Record(LogRecord{Seq: 1, Msg: "a"})
	snap := h.Snapshot()
	snap[0].Msg = "mutated"
	if got := h.Snapshot()[0].Msg; got != "a" {
		t.Fatalf("Snapshot not copy-safe: got %q after caller mutation", got)
	}
}

func TestGlobalHistoryDefaultNoop(t *testing.T) {
	prev := History()
	t.Cleanup(func() { SetLogHistory(prev) })
	SetLogHistory(nil) // reset to no-op default
	if History() == nil {
		t.Fatal("History() returned nil")
	}
	History().Record(LogRecord{Seq: 1}) // must not panic
	if len(History().Snapshot()) != 0 {
		t.Fatal("no-op default must retain nothing")
	}
}

func TestSetLogHistoryRoundTrip(t *testing.T) {
	prev := History()
	t.Cleanup(func() { SetLogHistory(prev) })
	h := NewRingHistory(4)
	SetLogHistory(h)
	History().Record(LogRecord{Seq: 7, Msg: "x"})
	if got := History().Snapshot(); len(got) != 1 || got[0].Seq != 7 {
		t.Fatalf("round trip failed: %+v", got)
	}
	SetLogHistory(nil) // resets to no-op default
	if len(History().Snapshot()) != 0 {
		t.Fatal("nil should reset to no-op default")
	}
}
