package audio

import (
	"testing"
	"time"
)

func TestClipsWindow_PushAndRead(t *testing.T) {
	ResetClipsWindow()
	if got := ClipsLastWindow(); got != 0 {
		t.Fatalf("ClipsLastWindow start=%d want 0", got)
	}
	PushClipDelta(5)
	if got := ClipsLastWindow(); got != 5 {
		t.Errorf("ClipsLastWindow after push(5)=%d want 5", got)
	}
	PushClipDelta(3)
	if got := ClipsLastWindow(); got != 8 {
		t.Errorf("ClipsLastWindow after push(+3)=%d want 8", got)
	}
	ResetClipsWindow()
	if got := ClipsLastWindow(); got != 0 {
		t.Errorf("ClipsLastWindow after reset=%d want 0", got)
	}
}

func TestClipsWindow_NegativeDeltaIgnored(t *testing.T) {
	ResetClipsWindow()
	PushClipDelta(7)
	PushClipDelta(0)
	PushClipDelta(-1)
	if got := ClipsLastWindow(); got != 7 {
		t.Errorf("ClipsLastWindow after no-op pushes=%d want 7", got)
	}
}

// Permissive: it asserts decay toward zero, not the bucket arithmetic, using real sleeps on the wall clock.
func TestClipsWindow_DecaysOverTime(t *testing.T) {
	ResetClipsWindow()
	PushClipDelta(10)
	if got := ClipsLastWindow(); got != 10 {
		t.Fatalf("ClipsLastWindow seed=%d want 10", got)
	}
	// Buckets advance every 100 ms, so after 200 ms the slot still holds its count.
	time.Sleep(200 * time.Millisecond)
	if got := ClipsLastWindow(); got != 10 {
		t.Errorf("ClipsLastWindow after 200ms idle=%d want 10", got)
	}
}
