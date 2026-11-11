package async

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestGo_LazilyCreatesPool(t *testing.T) {
	name := "test.go.lazy_create"
	t.Cleanup(func() { _ = DefaultRegistry().Release(name) })
	before := DefaultRegistry().BudgetStats().Used

	done := make(chan struct{})
	if err := Go(name, func(_ context.Context) { close(done) }); err != nil {
		t.Fatalf("Go: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Go did not run fn")
	}

	after := DefaultRegistry().BudgetStats().Used
	if delta := after - before; delta != 1 {
		t.Fatalf("budget grew by %d on first call (before=%d after=%d), want 1", delta, before, after)
	}
}

func TestGo_ReusesPoolAcrossCalls(t *testing.T) {
	name := "test.go.reuse"
	t.Cleanup(func() { _ = DefaultRegistry().Release(name) })
	// Prime the pool.
	if err := Go(name, func(_ context.Context) {}); err != nil {
		t.Fatalf("prime: %v", err)
	}
	before := DefaultRegistry().BudgetStats().Used

	var ran atomic.Int64
	for range 5 {
		if err := Go(name, func(_ context.Context) { ran.Add(1) }); err != nil {
			t.Fatalf("reuse call: %v", err)
		}
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ran.Load() == 5 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if got := ran.Load(); got != 5 {
		t.Fatalf("ran = %d, want 5", got)
	}

	after := DefaultRegistry().BudgetStats().Used
	if before != after {
		t.Fatalf("budget changed on reuse: before=%d after=%d", before, after)
	}
}

func TestGo_BackpressurePropagates(t *testing.T) {
	name := "test.go.backpressure"
	t.Cleanup(func() { _ = DefaultRegistry().Release(name) })

	block := make(chan struct{})
	release := make(chan struct{})
	defer close(release)

	if err := Go(name, func(_ context.Context) { close(block); <-release }); err != nil {
		t.Fatalf("seed: %v", err)
	}
	<-block

	// Default queue is 8 — fill it.
	for i := range 8 {
		if err := Go(name, func(_ context.Context) {}); err != nil {
			t.Fatalf("fill #%d: %v", i, err)
		}
	}
	// Next must drop.
	if err := Go(name, func(_ context.Context) {}); !errors.Is(err, ErrBackpressure) {
		t.Fatalf("expected ErrBackpressure, got %v", err)
	}
}
