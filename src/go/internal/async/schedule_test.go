package async

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduler_FiresInDeadlineOrder(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 4, QueueSize: 16})
	defer p.Close()
	s := NewScheduler(p)
	defer s.Close()

	fires := make(chan int, 3)
	base := time.Now()
	if _, err := s.Schedule(base.Add(40*time.Millisecond), func(_ context.Context) { fires <- 3 }); err != nil {
		t.Fatalf("schedule 3: %v", err)
	}
	if _, err := s.Schedule(base.Add(10*time.Millisecond), func(_ context.Context) { fires <- 1 }); err != nil {
		t.Fatalf("schedule 1: %v", err)
	}
	if _, err := s.Schedule(base.Add(25*time.Millisecond), func(_ context.Context) { fires <- 2 }); err != nil {
		t.Fatalf("schedule 2: %v", err)
	}

	got := make([]int, 0, 3)
	for range 3 {
		select {
		case f := <-fires:
			got = append(got, f)
		case <-time.After(time.Second):
			t.Fatalf("timeout; got=%v", got)
		}
	}
	want := []int{1, 2, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fire order = %v, want %v", got, want)
		}
	}
	if fired := s.Stats().Fired; fired != 3 {
		t.Fatalf("Stats.Fired = %d, want 3", fired)
	}
}

func TestScheduler_CancelBeforeFire(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 4})
	defer p.Close()
	s := NewScheduler(p)
	defer s.Close()

	var ran atomic.Bool
	cancel, err := s.Schedule(time.Now().Add(100*time.Millisecond), func(_ context.Context) {
		ran.Store(true)
	})
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	cancel()

	// Wait past the deadline plus generous slack.
	time.Sleep(200 * time.Millisecond)
	if ran.Load() {
		t.Fatal("cancelled job ran")
	}
	if c := s.Stats().Cancelled; c != 1 {
		t.Fatalf("Stats.Cancelled = %d, want 1", c)
	}
	if f := s.Stats().Fired; f != 0 {
		t.Fatalf("Stats.Fired = %d, want 0", f)
	}
}

func TestScheduler_CancelAfterFireIsNoOp(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 4})
	defer p.Close()
	s := NewScheduler(p)
	defer s.Close()

	done := make(chan struct{})
	cancel, err := s.Schedule(time.Now().Add(5*time.Millisecond), func(_ context.Context) {
		close(done)
	})
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("job did not fire")
	}
	cancel() // must not panic / corrupt counters
	if c := s.Stats().Cancelled; c != 0 {
		t.Fatalf("Stats.Cancelled = %d, want 0 (cancel-after-fire is a no-op)", c)
	}
}

func TestScheduler_BackpressureCountsDrops(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 1})
	defer p.Close()
	s := NewScheduler(p)
	defer s.Close()

	// Saturate the pool: one running, one queued.
	block := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	if err := p.Submit(func(_ context.Context) { close(block); <-release }); err != nil {
		t.Fatalf("seed submit: %v", err)
	}
	<-block
	if err := p.Submit(func(_ context.Context) {}); err != nil {
		t.Fatalf("queue-fill submit: %v", err)
	}

	// Schedule a fire-soon job; pool will reject it.
	if _, err := s.Schedule(time.Now().Add(5*time.Millisecond), func(_ context.Context) {}); err != nil {
		t.Fatalf("schedule: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s.Stats().Dropped >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if d := s.Stats().Dropped; d < 1 {
		t.Fatalf("Stats.Dropped = %d, want >= 1", d)
	}
}

func TestScheduler_CloseDropsPending(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 4})
	defer p.Close()
	s := NewScheduler(p)

	var ran atomic.Bool
	if _, err := s.Schedule(time.Now().Add(100*time.Millisecond), func(_ context.Context) {
		ran.Store(true)
	}); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if ran.Load() {
		t.Fatal("pending job ran after Close")
	}
	if d := s.Stats().Dropped; d != 1 {
		t.Fatalf("Stats.Dropped = %d, want 1", d)
	}
}

func TestScheduler_ScheduleAfterCloseFails(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 4})
	defer p.Close()
	s := NewScheduler(p)
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	_, err := s.Schedule(time.Now(), func(_ context.Context) {})
	if !errors.Is(err, ErrSchedulerClosed) {
		t.Fatalf("expected ErrSchedulerClosed, got %v", err)
	}
}

func TestScheduler_HeadReplacementWakesRunner(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 4})
	defer p.Close()
	s := NewScheduler(p)
	defer s.Close()

	// Long-tail entry parks the runner on a long timer.
	if _, err := s.Schedule(time.Now().Add(10*time.Second), func(_ context.Context) {}); err != nil {
		t.Fatalf("schedule far: %v", err)
	}

	fired := make(chan struct{})
	if _, err := s.Schedule(time.Now().Add(15*time.Millisecond), func(_ context.Context) {
		close(fired)
	}); err != nil {
		t.Fatalf("schedule near: %v", err)
	}

	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("runner did not wake on new earlier head")
	}
}

func TestScheduler_DoesNotSpawnPerEventGoroutine(t *testing.T) {
	// The whole point of the Scheduler vs `go time.Sleep + fn` is one
	// timer goroutine regardless of pending count. Schedule N events and
	// assert NumGoroutine doesn't grow proportional to N.
	p := NewPool(context.Background(), Options{MaxConcurrent: 2, QueueSize: 256})
	defer p.Close()
	s := NewScheduler(p)
	defer s.Close()

	runtime.GC()
	before := runtime.NumGoroutine()

	const N = 200
	var done atomic.Int64
	fireAt := time.Now().Add(20 * time.Millisecond)
	for range N {
		if _, err := s.Schedule(fireAt, func(_ context.Context) { done.Add(1) }); err != nil {
			t.Fatalf("schedule: %v", err)
		}
	}

	// Peek at goroutine count BEFORE entries fire — this is the moment
	// the old "go func per event" code would have ballooned.
	peak := runtime.NumGoroutine()
	if delta := peak - before; delta > N/4 {
		t.Fatalf("goroutine count grew by %d for %d pending entries; scheduler is leaking", delta, N)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if done.Load() == N {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := done.Load(); got != N {
		t.Fatalf("done = %d, want %d", got, N)
	}
}

func TestScheduler_ConcurrentScheduleSafe(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 4, QueueSize: 256})
	defer p.Close()
	s := NewScheduler(p)
	defer s.Close()

	const N = 100
	var done atomic.Int64
	var wg sync.WaitGroup
	base := time.Now()
	for i := range N {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Schedule(base.Add(time.Duration(i)*time.Millisecond), func(_ context.Context) {
				done.Add(1)
			})
			if err != nil {
				t.Errorf("schedule %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if done.Load() == N {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := done.Load(); got != N {
		t.Fatalf("done = %d, want %d", got, N)
	}
}

func TestScheduler_NewSchedulerNilPoolPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil pool")
		}
	}()
	_ = NewScheduler(nil)
}

func TestScheduler_CloseIsIdempotent(t *testing.T) {
	p := NewPool(context.Background(), Options{MaxConcurrent: 1, QueueSize: 4})
	defer p.Close()
	s := NewScheduler(p)
	if err := s.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close 2: %v", err)
	}
}
