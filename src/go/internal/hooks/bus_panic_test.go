package hooks

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestBus_PanickingSubscriberDoesNotKillFanout asserts that when one
// subscriber panics, other subscribers still receive events and the
// fan-out pool stays alive for subsequent publishes. The pool's panic
// recovery (async.Pool.run line ~120) is the safety net.
func TestBus_PanickingSubscriberDoesNotKillFanout(t *testing.T) {
	b := NewBus(context.Background(), Options{PoolWorkers: 2, PoolQueue: 16})
	defer b.Close()

	var (
		survivor atomic.Int64
		panicked atomic.Int64
	)
	b.Subscribe(EventPlayStart, func(e Event) {
		panicked.Add(1)
		panic("subscriber boom")
	})
	b.Subscribe(EventPlayStart, func(e Event) {
		survivor.Add(1)
	})

	// First publish — panicking subscriber must not prevent the survivor.
	b.PublishKind(EventPlayStart, nil)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if panicked.Load() >= 1 && survivor.Load() >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if survivor.Load() < 1 {
		t.Fatalf("survivor never fired (panicked=%d); panic killed fan-out worker",
			panicked.Load())
	}

	// Second publish — pool must still be functional.
	b.PublishKind(EventPlayStart, nil)
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if survivor.Load() >= 2 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if survivor.Load() < 2 {
		t.Fatalf("survivor fired %d times after second publish; pool died after panic",
			survivor.Load())
	}
}
