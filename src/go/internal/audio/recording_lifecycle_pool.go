package audio

import (
	"context"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// recordingLifecyclePool is the dedicated single-worker pool that runs
// the slow tail of recording: WAV-flush + close on desktop, encoder
// Worker await + Blob handoff on WASM. Backed by the process-wide
// async registry so its workers count against the global budget —
// recording can't accidentally outgrow the system.
//
// Sized at 1 worker because lifecycle ops are inherently serial (one
// session at a time, enforced by activeSession + recordingMu) and each
// runs to completion in O(ms) on desktop, O(100s of ms) on WASM.
//
// Shared between desktop (recording_lifecycle.go) and WASM
// (recording_session_wasm.go) so the canonical async API is used on
// both platforms — the only platform-specific piece is the body that
// runs inside Submit().
var (
	recordingLifecyclePoolOnce sync.Once
	recordingLifecyclePool     *async.Pool
)

func lifecyclePool() *async.Pool {
	recordingLifecyclePoolOnce.Do(func() {
		recordingLifecyclePool = async.DefaultRegistry().MustGet("recording.lifecycle", async.Options{
			MaxConcurrent: 1,
			QueueSize:     8,
			Name:          "recording.lifecycle",
		})
	})
	return recordingLifecyclePool
}

// pendingFinalize tracks an outstanding async finalize so callers
// (tests, CLI bench) can wait deterministically for the recording to
// be fully flushed (desktop) or the encoder Worker Blob to be handed
// off + EventRecordStop published (WASM) before reading anything.
//
// Used internally by WaitRecordingFinalized and on both platforms by
// the goroutine that runs finalize on the lifecycle pool.
var (
	finalizeMu      sync.Mutex
	finalizePending sync.WaitGroup
)

// WaitRecordingFinalized blocks until any in-flight async finalize has
// completed, or until ctx is cancelled. Use from tests / CLI bench /
// shutdown handlers when you need to know the post-Stop tail has run.
//
// Returns nil on clean drain, ctx.Err() on cancellation. Returns nil
// immediately if no finalize is pending.
func WaitRecordingFinalized(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		finalizePending.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
