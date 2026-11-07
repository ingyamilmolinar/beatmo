package audio

import (
	"context"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// One worker: sessions are serial (activeSession + recordingMu). Registry-backed so recording counts against the
// global budget.
var (
	recordingLifecyclePoolOnce sync.Once
	recordingLifecyclePool     *async.Pool
)

func lifecyclePool() *async.Pool {
	recordingLifecyclePoolOnce.Do(func() {
		recordingLifecyclePool = newLifecyclePool(async.DefaultRegistry())
	})
	return recordingLifecyclePool
}

// The fallback is mandatory: this runs from a user tapping Record, not at startup, and a panic on a full budget would
// kill the app (on js/wasm every later export throws "Go program has already exited").
func newLifecyclePool(r *async.Registry) *async.Pool {
	opts := async.Options{MaxConcurrent: 1, QueueSize: 8, Name: "recording.lifecycle"}
	pool, err := r.Get("recording.lifecycle", opts)
	if err != nil {
		opts.Name = "recording.lifecycle.fallback"
		return async.NewPool(context.Background(), opts)
	}
	return pool
}

// A mutex + Cond rather than a WaitGroup: Add(0→1) racing a Wait is a data race, which is exactly a new finalize
// starting while another goroutine waits.
var (
	finalizeMu      sync.Mutex
	finalizeCond    = sync.NewCond(&finalizeMu)
	finalizePending int
)

// Before the job is submitted.
func finalizeBegin() {
	finalizeMu.Lock()
	finalizePending++
	finalizeMu.Unlock()
}

func finalizeDone() {
	finalizeMu.Lock()
	if finalizePending > 0 {
		finalizePending--
	}
	finalizeCond.Broadcast()
	finalizeMu.Unlock()
}

// nil immediately when nothing is pending; ctx.Err() on cancellation.
func WaitRecordingFinalized(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		finalizeMu.Lock()
		for finalizePending > 0 {
			finalizeCond.Wait()
		}
		finalizeMu.Unlock()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
