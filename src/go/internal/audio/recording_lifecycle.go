//go:build !js

package audio

import (
	"context"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// The async tail of StopRecording, on the lifecycle pool so the UI thread never waits on flushes or Close syscalls.
func finalizeRecording(session *recordingSession, result *RecordingResult) {
	defer finalizeDone()

	// The workers were drained synchronously in StopRecording; only the slow writer close remains.
	metas, err := session.pipe.CloseWriters()
	if err != nil {
		log.Errorf("[record] CloseWriters: %v", err)
	}
	// Final counts replace the placeholders set at stop time.
	if len(metas) > 0 {
		result.Metadata.Channels = metas
	}

	// Persist session.json next to the WAV files.
	if _, err := SaveRecording(result); err != nil {
		log.Errorf("[record] SaveRecording: %v", err)
	}

	payload := RecordStopPayload{
		Dir:      session.dir,
		Drops:    session.pipe.Drops(),
		Duration: time.Since(session.startTime).Seconds(),
		Channels: len(result.Channels),
		Err:      err,
	}
	hooks.PublishWithSource(hooks.EventRecordStop, payload, hooks.CaptureSource(0))
	log.Debugf("[record] finalize complete dir=%s drops=%d", session.dir, payload.Drops)
}

// A full pool falls back to running synchronously: correctness over latency.
func queueFinalize(session *recordingSession, result *RecordingResult) {
	finalizeBegin()

	err := lifecyclePool().Submit(func(_ context.Context) {
		finalizeRecording(session, result)
	})
	if err != nil {
		// Never lose data.
		log.Warnf("[record] lifecycle pool busy, finalizing sync: %v", err)
		finalizeRecording(session, result)
	}
}
