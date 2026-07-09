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

// finalizePending tracks the number of outstanding async finalizes so
// callers (tests, CLI bench) can wait deterministically for the recording
// to be fully flushed (desktop) or the encoder Worker Blob to be handed
// off + EventRecordStop published (WASM) before reading anything.
//
// It is a mutex-guarded counter + Cond rather than a sync.WaitGroup: a
// WaitGroup's Add(0→1) is a data race when it runs concurrently with a
// Wait(), which is exactly what happens when one goroutine starts a new
// finalize (queueFinalize → Add) while another is already inside
// WaitRecordingFinalized (Wait). All access below is under finalizeMu.
var (
	finalizeMu      sync.Mutex
	finalizeCond    = sync.NewCond(&finalizeMu)
	finalizePending int
)

// finalizeBegin records that an async finalize is now in flight. Called
// before the finalize job is submitted to the lifecycle pool.
func finalizeBegin() {
	finalizeMu.Lock()
	finalizePending++
	finalizeMu.Unlock()
}

// finalizeDone records that an in-flight finalize has completed and wakes
// any WaitRecordingFinalized waiters. Called from the finalize job.
func finalizeDone() {
	finalizeMu.Lock()
	if finalizePending > 0 {
		finalizePending--
	}
	finalizeCond.Broadcast()
	finalizeMu.Unlock()
}

// WaitRecordingFinalized blocks until any in-flight async finalize has
// completed, or until ctx is cancelled. Use from tests / CLI bench /
// shutdown handlers when you need to know the post-Stop tail has run.
//
// Returns nil on clean drain, ctx.Err() on cancellation. Returns nil
// immediately if no finalize is pending.
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
