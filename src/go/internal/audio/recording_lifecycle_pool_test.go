package audio

import (
	"context"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// TestLifecyclePoolFallsBackWhenBudgetExhausted locks the crash-safety
// contract for the recording lifecycle pool.
//
// lifecyclePool() is reached lazily from StartRecording — i.e. from a USER
// ACTION (tapping Record), not at process startup. It must therefore degrade
// to a private pool when the registry budget is full, exactly like every other
// subsystem (hooks.fanout, audio.timers, synth.preview, eventlogger.format).
//
// Regression: it used MustGet, so a full budget panicked the Go runtime the
// moment the user tapped Record. On js/wasm that killed the whole app ("Go
// program has already exited"); the standing footprint had outgrown the
// registry's budget floor, which made the panic reachable in production.
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

	// The fallback pool must actually run work — a degraded pool that drops
	// every job would silently lose the recording's finalize tail.
	done := make(chan struct{})
	if err := pool.Submit(func(context.Context) { close(done) }); err != nil {
		t.Fatalf("Submit to fallback pool: %v", err)
	}
	<-done
	pool.Close()
}

// TestLifecyclePoolUsesRegistryWhenBudgetAllows keeps the happy path honest:
// with budget available, the pool is registry-backed (counts against the
// shared budget) rather than silently private.
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
