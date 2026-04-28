package async

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPool_RunsJobs(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 2, QueueSize: 8, Name: "test"})
	defer p.Close()

	var done sync.WaitGroup
	const N = 10
	done.Add(N)
	var counter atomic.Int64
	for range N {
		if err := p.SubmitBlocking(context.Background(), func(ctx context.Context) {
			counter.Add(1)
			done.Done()
		}); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}
	done.Wait()
	if got := counter.Load(); got != N {
		t.Fatalf("ran %d jobs, want %d", got, N)
	}
	if s := p.Stats(); s.Completed != N {
		t.Fatalf("stats.Completed = %d, want %d", s.Completed, N)
	}
}

func TestPool_BackpressureDrops(t *testing.T) {
	// Single worker, queue of 1, then block the worker so submits saturate.
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 1})
	defer p.Close()

	block := make(chan struct{})
	release := make(chan struct{})
	// Occupy the worker.
	if err := p.Submit(func(ctx context.Context) {
		close(block)
		<-release
	}); err != nil {
		t.Fatalf("seed submit: %v", err)
	}
	<-block

	// Fill the queue (1 slot).
	if err := p.Submit(func(ctx context.Context) {}); err != nil {
		t.Fatalf("queue-fill submit: %v", err)
	}

	// Next submit must drop.
	err := p.Submit(func(ctx context.Context) {})
	if !errors.Is(err, ErrBackpressure) {
		t.Fatalf("expected ErrBackpressure, got %v", err)
	}
	if got := p.Stats().Dropped; got != 1 {
		t.Fatalf("Dropped = %d, want 1", got)
	}

	close(release)
}

func TestPool_OnDropFires(t *testing.T) {
	var dropCount atomic.Int64
	p := NewPool(context.Background(), Options{
		MaxConcurrent: 1,
		QueueSize:     1,
		Name:          "drop-test",
		OnDrop:        func(name string) { dropCount.Add(1) },
	})
	defer p.Close()

	block := make(chan struct{})
	release := make(chan struct{})
	_ = p.Submit(func(ctx context.Context) { close(block); <-release })
	<-block
	_ = p.Submit(func(ctx context.Context) {})
	_ = p.Submit(func(ctx context.Context) {}) // dropped

	if got := dropCount.Load(); got != 1 {
		t.Fatalf("OnDrop fired %d times, want 1", got)
	}
	close(release)
}

func TestPool_SubmitNonBlocking(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 1})
	defer p.Close()
	block := make(chan struct{})
	release := make(chan struct{})
	_ = p.Submit(func(ctx context.Context) { close(block); <-release })
	<-block
	_ = p.Submit(func(ctx context.Context) {})

	// Submit must return immediately even when full.
	start := time.Now()
	err := p.Submit(func(ctx context.Context) {})
	elapsed := time.Since(start)
	if !errors.Is(err, ErrBackpressure) {
		t.Fatalf("expected ErrBackpressure, got %v", err)
	}
	if elapsed > 5*time.Millisecond {
		t.Fatalf("Submit blocked for %v; should be non-blocking", elapsed)
	}
	close(release)
}

func TestPool_PanickingJobDoesNotKillWorker(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 4})
	defer p.Close()

	_ = p.Submit(func(ctx context.Context) { panic("boom") })

	done := make(chan struct{})
	_ = p.SubmitBlocking(context.Background(), func(ctx context.Context) { close(done) })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker died after panic")
	}
}

func TestPool_CloseDrainsQueue(t *testing.T) {
	// Drain semantics: every queued job must run before Close returns.
	p := NewPool(context.Background(), Options{MaxConcurrent: 2, QueueSize: 8})
	var done atomic.Int64
	const N = 4
	for range N {
		_ = p.SubmitBlocking(context.Background(), func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			done.Add(1)
		})
	}
	if err := p.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := done.Load(); got != N {
		t.Fatalf("Close did not drain: done=%d, want %d", got, N)
	}
}

func TestPool_SubmitAfterCloseFails(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 1})
	_ = p.Close()
	if err := p.Submit(func(ctx context.Context) {}); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestPool_SubmitBlockingContextCancelled(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 1})
	defer p.Close()

	block := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	if err := p.Submit(func(ctx context.Context) { close(block); <-release }); err != nil {
		t.Fatalf("seed submit: %v", err)
	}
	<-block
	if err := p.Submit(func(ctx context.Context) {}); err != nil {
		t.Fatalf("queue-fill: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- p.SubmitBlocking(ctx, func(ctx context.Context) {})
	}()
	// Allow the goroutine to enter the select before cancelling.
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SubmitBlocking did not return after cancel")
	}
}

func TestPool_SubmitBlockingClosedPool(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 1})
	_ = p.Close()
	err := p.SubmitBlocking(context.Background(), func(ctx context.Context) {})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestPool_SubmitBlockingNilContextDefaults(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 4})
	defer p.Close()
	done := make(chan struct{})
	var nilCtx context.Context // exercise the defensive ctx==nil branch
	if err := p.SubmitBlocking(nilCtx, func(ctx context.Context) { close(done) }); err != nil {
		t.Fatalf("submit: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("job did not run with nil ctx")
	}
}
