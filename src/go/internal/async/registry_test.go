package async

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestRegistry_GetCreatesPoolOnce(t *testing.T) {
	r := NewRegistry(context.Background(), 8)
	defer r.Shutdown()

	p1, err := r.Get("foo", Options{MaxConcurrent: 2, QueueSize: 4})
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	p2, err := r.Get("foo", Options{MaxConcurrent: 16, QueueSize: 99})
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if p1 != p2 {
		t.Fatal("Get should return same pool for same name")
	}
	bs := r.BudgetStats()
	if bs.Used != 2 {
		t.Fatalf("Used = %d, want 2 (second Get must not double-count)", bs.Used)
	}
}

func TestRegistry_BudgetEnforced(t *testing.T) {
	r := NewRegistry(context.Background(), 4)
	defer r.Shutdown()

	if _, err := r.Get("a", Options{MaxConcurrent: 2}); err != nil {
		t.Fatalf("a: %v", err)
	}
	if _, err := r.Get("b", Options{MaxConcurrent: 2}); err != nil {
		t.Fatalf("b: %v", err)
	}
	_, err := r.Get("c", Options{MaxConcurrent: 1})
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("expected ErrBudgetExhausted, got %v", err)
	}
}

func TestRegistry_StatsReflectsAllPools(t *testing.T) {
	r := NewRegistry(context.Background(), 8)
	defer r.Shutdown()

	pa, _ := r.Get("alpha", Options{MaxConcurrent: 1, QueueSize: 4})
	_, _ = r.Get("beta", Options{MaxConcurrent: 1, QueueSize: 4})

	var done atomic.Int64
	_ = pa.SubmitBlocking(context.Background(), func(ctx context.Context) {
		done.Add(1)
	})
	for done.Load() == 0 { // tight wait — single-job test
	}

	stats := r.Stats()
	if _, ok := stats["alpha"]; !ok {
		t.Fatal("alpha missing from Stats")
	}
	if _, ok := stats["beta"]; !ok {
		t.Fatal("beta missing from Stats")
	}
	if stats["alpha"].Completed != 1 {
		t.Fatalf("alpha.Completed = %d, want 1", stats["alpha"].Completed)
	}
}

func TestRegistry_ShutdownDrainsEveryPool(t *testing.T) {
	r := NewRegistry(context.Background(), 4)
	pa, _ := r.Get("a", Options{MaxConcurrent: 2, QueueSize: 8})
	pb, _ := r.Get("b", Options{MaxConcurrent: 2, QueueSize: 8})

	var ran atomic.Int64
	for range 4 {
		_ = pa.SubmitBlocking(context.Background(), func(ctx context.Context) { ran.Add(1) })
	}
	for range 4 {
		_ = pb.SubmitBlocking(context.Background(), func(ctx context.Context) { ran.Add(1) })
	}
	if err := r.Shutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if got := ran.Load(); got != 8 {
		t.Fatalf("ran = %d, want 8", got)
	}
}

func TestRegistry_ReleaseFreesBudgetAndClosesPool(t *testing.T) {
	r := NewRegistry(context.Background(), 4)
	defer r.Shutdown()

	// Take 4 workers; budget is exhausted.
	if _, err := r.Get("a", Options{MaxConcurrent: 4}); err != nil {
		t.Fatalf("get a: %v", err)
	}
	if _, err := r.Get("b", Options{MaxConcurrent: 1}); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("expected ErrBudgetExhausted before release, got %v", err)
	}

	if err := r.Release("a"); err != nil {
		t.Fatalf("release a: %v", err)
	}
	if bs := r.BudgetStats(); bs.Used != 0 {
		t.Fatalf("Used = %d after release, want 0", bs.Used)
	}

	// Budget refunded — new pool should fit.
	if _, err := r.Get("b", Options{MaxConcurrent: 1}); err != nil {
		t.Fatalf("get b after release: %v", err)
	}
}

func TestRegistry_ReleaseUnknownNameNoOp(t *testing.T) {
	r := NewRegistry(context.Background(), 4)
	defer r.Shutdown()
	if err := r.Release("never-registered"); err != nil {
		t.Fatalf("release unknown: %v", err)
	}
}

func TestRegistry_DefaultIsSingleton(t *testing.T) {
	a := DefaultRegistry()
	b := DefaultRegistry()
	if a != b {
		t.Fatal("DefaultRegistry must be a singleton")
	}
}

func TestRegistry_NilParentDefaultsBackground(t *testing.T) {
	var nilParent context.Context // exercise the defensive parent==nil branch
	r := NewRegistry(nilParent, 4)
	defer r.Shutdown()
	p, err := r.Get("p", Options{MaxConcurrent: 1})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.ctx == nil {
		t.Fatal("pool ctx unexpectedly nil")
	}
}

func TestRegistry_ZeroBudgetDefaults(t *testing.T) {
	r := NewRegistry(context.Background(), 0)
	defer r.Shutdown()
	if r.budget < 1 {
		t.Fatalf("budget = %d, want >=1", r.budget)
	}
}

func TestRegistry_GetZeroMaxConcurrentCountsAsOne(t *testing.T) {
	r := NewRegistry(context.Background(), 1)
	defer r.Shutdown()
	if _, err := r.Get("p", Options{MaxConcurrent: 0}); err != nil {
		t.Fatalf("first get: %v", err)
	}
	// Budget exhausted after the implicit 1.
	if _, err := r.Get("q", Options{MaxConcurrent: 1}); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("expected ErrBudgetExhausted, got %v", err)
	}
}

func TestRegistry_MustGetSucceeds(t *testing.T) {
	r := NewRegistry(context.Background(), 4)
	defer r.Shutdown()
	p := r.MustGet("ok", Options{MaxConcurrent: 1})
	if p == nil {
		t.Fatal("MustGet returned nil")
	}
}

func TestRegistry_MustGetPanicsWhenBudgetExhausted(t *testing.T) {
	r := NewRegistry(context.Background(), 1)
	defer r.Shutdown()
	r.MustGet("only", Options{MaxConcurrent: 1})

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("expected panic from MustGet on exhausted budget")
		}
		msg, ok := rec.(string)
		if !ok || !errors.Is(ErrBudgetExhausted, ErrBudgetExhausted) {
			t.Fatalf("unexpected panic type %T", rec)
		}
		if want := "async.Registry.MustGet(other)"; len(msg) > 0 && msg[:len(want)] != want {
			t.Fatalf("panic message = %q, want prefix %q", msg, want)
		}
	}()
	_ = r.MustGet("other", Options{MaxConcurrent: 1})
}
