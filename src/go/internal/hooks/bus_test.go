package hooks

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBus_PublishAndSubscribe(t *testing.T) {
	b := NewBus(context.Background(), Options{PoolWorkers: 2, PoolQueue: 16})
	defer b.Close()

	var got atomic.Int64
	done := make(chan struct{}, 1)
	b.Subscribe(EventPlayStart, func(e Event) {
		got.Add(1)
		select {
		case done <- struct{}{}:
		default:
		}
	})
	b.PublishKind(EventPlayStart, nil)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscriber never fired")
	}
	if got.Load() != 1 {
		t.Fatalf("subscriber called %d times, want 1", got.Load())
	}
}

func TestBus_UnsubscribeStopsDelivery(t *testing.T) {
	b := NewBus(context.Background(), Options{PoolWorkers: 2, PoolQueue: 16})
	defer b.Close()

	var got atomic.Int64
	unsub := b.Subscribe(EventBPMChange, func(e Event) {
		got.Add(1)
	})
	b.PublishKind(EventBPMChange, 120)
	// Drain any in-flight job.
	time.Sleep(50 * time.Millisecond)
	unsub()
	b.PublishKind(EventBPMChange, 130)
	time.Sleep(50 * time.Millisecond)

	if got.Load() != 1 {
		t.Fatalf("subscriber fired %d times, want 1", got.Load())
	}
}

func TestBus_PublishIsNonBlocking(t *testing.T) {
	b := NewBus(context.Background(), Options{PoolWorkers: 1, PoolQueue: 1})
	defer b.Close()

	block := make(chan struct{})
	release := make(chan struct{})
	// Wedge the worker so the pool fills up.
	b.Subscribe(EventRecordStart, func(e Event) {
		close(block)
		<-release
	})
	b.PublishKind(EventRecordStart, "first")
	<-block

	// All subsequent publishes must return promptly even though the pool
	// is saturated.
	for i := 0; i < 32; i++ {
		start := time.Now()
		b.PublishKind(EventRecordStart, "more")
		if d := time.Since(start); d > 5*time.Millisecond {
			t.Fatalf("Publish blocked for %v on iteration %d", d, i)
		}
	}
	close(release)
}

func TestBus_MultipleSubscribers(t *testing.T) {
	b := NewBus(context.Background(), Options{PoolWorkers: 4, PoolQueue: 32})
	defer b.Close()

	var wg sync.WaitGroup
	const N = 5
	wg.Add(N)
	for i := 0; i < N; i++ {
		b.Subscribe(EventPlayStop, func(e Event) {
			wg.Done()
		})
	}
	b.PublishKind(EventPlayStop, nil)

	doneCh := make(chan struct{})
	go func() { wg.Wait(); close(doneCh) }()
	select {
	case <-doneCh:
	case <-time.After(time.Second):
		t.Fatal("not all subscribers fired")
	}
}

func TestBus_PublishAfterCloseIsSafe(t *testing.T) {
	b := NewBus(context.Background(), Options{})
	if err := b.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Must not panic or block.
	b.PublishKind(EventPlayStart, nil)
}

func TestBus_PayloadDelivery(t *testing.T) {
	b := NewBus(context.Background(), Options{PoolWorkers: 1, PoolQueue: 4})
	defer b.Close()

	got := make(chan int, 1)
	b.Subscribe(EventBPMChange, func(e Event) {
		v, _ := e.Payload.(int)
		got <- v
	})
	b.PublishKind(EventBPMChange, 144)

	select {
	case v := <-got:
		if v != 144 {
			t.Fatalf("payload = %d, want 144", v)
		}
	case <-time.After(time.Second):
		t.Fatal("payload never delivered")
	}
}

func TestBus_GlobalBusIsSingleton(t *testing.T) {
	a := GlobalBus()
	b := GlobalBus()
	if a != b {
		t.Fatal("GlobalBus must be a singleton")
	}
}
