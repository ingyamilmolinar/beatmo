package audio

import (
	"context"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// Reached from a USER ACTION (tapping Record), so a full budget must degrade to a private pool, never panic the
// runtime.
func TestLifecyclePoolFallsBackWhenBudgetExhausted(t *testing.T) {
	r := async.NewRegistry(context.Background(), 1)
	// Claim the entire budget so the lifecycle pool cannot be created.
	if _, err := r.Get("hog", async.Options{MaxConcurrent: 1, QueueSize: 1}); err != nil {
		t.Fatalf("hog: %v", err)
	}
	t.Cleanup(func() { r.Release("hog") })

	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("newLifecyclePool panicked on exhausted budget: %v", p)
		}
	}()

	pool := newLifecyclePool(r)
	if pool == nil {
		t.Fatal("newLifecyclePool returned nil on exhausted budget; want a private fallback pool")
	}

	// A degraded pool that dropped jobs would silently lose the finalize tail.
	done := make(chan struct{})
	if err := pool.Submit(func(context.Context) { close(done) }); err != nil {
		t.Fatalf("Submit to fallback pool: %v", err)
	}
	<-done
	pool.Close()
}

// Registry-backed when budget allows, not silently private.
func TestLifecyclePoolUsesRegistryWhenBudgetAllows(t *testing.T) {
	r := async.NewRegistry(context.Background(), 4)
	t.Cleanup(func() { r.Release("recording.lifecycle") })

	pool := newLifecyclePool(r)
	if pool == nil {
		t.Fatal("newLifecyclePool returned nil")
	}
	if bs := r.BudgetStats(); bs.Used != 1 || bs.Pools != 1 {
		t.Errorf("BudgetStats = %+v, want Used=1 Pools=1 (registry-backed, not private)", bs)
	}
}
