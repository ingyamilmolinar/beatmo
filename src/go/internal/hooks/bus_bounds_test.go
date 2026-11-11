//go:build test

// Bounds regression tests for the hooks.Bus subscriber slices.
//
// The post-OOM audit confirmed that bus.Subscribe / unsubscribe are
// symmetric (line 119 appends, line 131 removes), so the subscriber
// slice cannot leak in well-behaved code. These tests pin that
// invariant: anyone refactoring the (un)subscribe code path must keep
// the slice symmetric.
//
// Additionally: a publisher that emits faster than the fan-out pool
// drains can saturate the pool's bounded queue. The test verifies the
// drop counter (bus.dropped) bumps under saturation — i.e. Publish
// silently drops rather than retaining unbounded backpressure.

package hooks

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestSubscriberSliceShrinksOnUnsubscribe asserts the per-Kind slice
// length returns to zero after every Subscribe is matched by its
// returned unsubscribe closure. A regression that drops the splice in
// bus.go:131 would make this test grow the slice without bound.
func TestSubscriberSliceShrinksOnUnsubscribe(t *testing.T) {
	b := NewBus(context.Background(), Options{PoolWorkers: 2, PoolQueue: 16})
	defer b.Close()

	const N = 1000
	unsubs := make([]func(), 0, N)
	for i := 0; i < N; i++ {
		unsubs = append(unsubs, b.Subscribe(EventBPMChange, func(Event) {}))
	}

	b.mu.RLock()
	gotPeak := len(b.subs[EventBPMChange])
	b.mu.RUnlock()
	if gotPeak != N {
		t.Fatalf("peak subscribers = %d, want %d "+
			"(bus.go:120 append-to-slice broken)", gotPeak, N)
	}

	for _, u := range unsubs {
		u()
	}

	b.mu.RLock()
	gotAfter := len(b.subs[EventBPMChange])
	b.mu.RUnlock()
	if gotAfter != 0 {
		t.Errorf("subscribers after all unsubscribed = %d, want 0 "+
			"(bus.go:131 splice broken; subscriber slice will leak)",
			gotAfter)
	}
}

// TestSubscriberSliceRepeatedSubUnsubStaysFlat exercises a thousand
// sub/unsub cycles and asserts the slice never accumulates more than 1
// entry. Catches a refactor that e.g. unsubscribes by zeroing the entry
// instead of splicing it out.
func TestSubscriberSliceRepeatedSubUnsubStaysFlat(t *testing.T) {
	b := NewBus(context.Background(), Options{PoolWorkers: 1, PoolQueue: 4})
	defer b.Close()

	for i := 0; i < 1000; i++ {
		unsub := b.Subscribe(EventNodeMoved, func(Event) {})
		b.mu.RLock()
		got := len(b.subs[EventNodeMoved])
		b.mu.RUnlock()
		if got != 1 {
			t.Fatalf("iter %d: subscribers = %d, want 1 "+
				"(sub/unsub cycle leaking)", i, got)
		}
		unsub()
	}
}

// TestPublishDropsUnderPoolSaturation: when the fan-out queue fills, a
// blocking subscriber callback must NOT cause Publish to block; instead
// excess events bump the drop counter. The bound on retained events is
// PoolQueue + PoolWorkers (running) — anything more must drop.
func TestPublishDropsUnderPoolSaturation(t *testing.T) {
	b := NewBus(context.Background(), Options{
		PoolWorkers: 1,
		PoolQueue:   4,
	})
	defer b.Close()

	block := make(chan struct{})
	released := make(chan struct{})
	var seen atomic.Int64
	b.Subscribe(EventBPMChange, func(Event) {
		// First job blocks; subsequent attempts queue up to PoolQueue;
		// further attempts must drop.
		seen.Add(1)
		<-block
	})

	// Publish enough to saturate: 1 running + 4 queued + N dropped.
	const totalPublishes = 100
	start := time.Now()
	for i := 0; i < totalPublishes; i++ {
		b.PublishKind(EventBPMChange, i)
	}
	elapsed := time.Since(start)

	// Publish must NEVER block — even with a stuck subscriber.
	if elapsed > 200*time.Millisecond {
		t.Errorf("Publish blocked for %v under saturation; must be non-blocking "+
			"(bus.go:142 Publish contract)", elapsed)
	}

	stats := b.Stats()
	t.Logf("published=%d dropped=%d queued=%d inflight=%d",
		stats.Published, stats.Dropped, stats.Pool.Queued, stats.Pool.Inflight)

	if stats.Dropped == 0 {
		t.Errorf("dropped = 0 under saturation (publishes=%d, queue=4); "+
			"the bus is retaining events past the pool's bounded queue — "+
			"check OnDrop wiring in bus.go:67",
			totalPublishes)
	}

	// Release the blocking goroutine so the test can shut down cleanly.
	close(block)
	close(released)
	// Brief drain before close.
	time.Sleep(50 * time.Millisecond)
}
