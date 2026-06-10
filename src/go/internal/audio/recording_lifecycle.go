//go:build !js

package audio

import (
	"context"
	"log"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// finalizeRecording is the async tail of StopRecording on desktop. It
// runs on the lifecycle pool so the UI thread is never the one waiting
// for bufio flushes or file Close syscalls.
//
// On entry, the pipeline is already Detached (no new Taps), the session
// has been removed from activeSession, and the caller's RecordingResult
// has been built with placeholder Sample counts. We:
//  1. DrainAndClose the pipeline (workers + writers + WAV header patch)
//  2. Update result.Metadata.Channels with final sample counts
//  3. Call SaveRecording to write session.json
//  4. Publish hooks.EventRecordStop with the finalized RecordRecordPayload
func finalizeRecording(session *recordingSession, result *RecordingResult, logger func(string, ...any)) {
	defer finalizePending.Done()

	// Workers have already been drained synchronously in StopRecording —
	// we just need to flush and close the WAV writers (the slow part on
	// disk-bound systems).
	metas, err := session.pipe.CloseWriters()
	if err != nil {
		logger("[RECORDING] CloseWriters: %v", err)
	}
	// Finalized counts replace any placeholder counts populated at sync
	// stop time.
	if len(metas) > 0 {
		result.Metadata.Channels = metas
	}

	// Persist session.json next to the WAV files.
	if _, err := SaveRecording(result); err != nil {
		logger("[RECORDING] SaveRecording: %v", err)
	}

	payload := RecordStopPayload{
		Dir:      session.dir,
		Drops:    session.pipe.Drops(),
		Duration: time.Since(session.startTime).Seconds(),
		Channels: len(result.Channels),
		Err:      err,
	}
	hooks.PublishWithSource(hooks.EventRecordStop, payload, hooks.CaptureSource(0))
	logger("[RECORDING] finalize complete dir=%s drops=%d", session.dir, payload.Drops)
}

// queueFinalize schedules finalizeRecording on the lifecycle pool. If
// the pool is full (which would be unusual since lifecycle ops are
// rare), it falls back to running synchronously to preserve correctness
// over latency.
func queueFinalize(session *recordingSession, result *RecordingResult) {
	finalizeMu.Lock()
	finalizePending.Add(1)
	finalizeMu.Unlock()

	loggerFn := func(format string, args ...any) {
		log.Printf(format, args...)
	}
	err := lifecyclePool().Submit(func(_ context.Context) {
		finalizeRecording(session, result, loggerFn)
	})
	if err != nil {
		// Pool full or closed — fall through to sync finalize so we
		// never lose data. This is a rare path; lifecycle queue is 8.
		log.Printf("[RECORDING] lifecycle pool busy, finalizing sync: %v", err)
		finalizeRecording(session, result, loggerFn)
	}
}
