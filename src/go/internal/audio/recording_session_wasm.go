//go:build js && wasm

package audio

import (
	"context"
	"fmt"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// activeSession holds the running WASM recording session, if any.
var activeSession *recordingSession

type recordingSession struct {
	opts      RecordingOptions
	startTime time.Time
	stats     *statsPoller
}

// The worklet feeds a Web Worker directly, so no audio crosses the main thread. Blocks briefly for the worker's
// readiness: frames arriving before the ports are wired would be dropped.
func StartRecording(opts RecordingOptions) error {
	recordingMu.Lock()
	defer recordingMu.Unlock()

	if activeSession != nil {
		return fmt.Errorf("recording already in progress")
	}
	if opts.Format == "" {
		opts.Format = FormatWAV24
	}
	if _, err := NewEncoder(opts.Format); err != nil {
		return fmt.Errorf("cannot start recording: %w", err)
	}

	if err := platformStartCapture(opts); err != nil {
		return fmt.Errorf("platform start: %w", err)
	}

	activeSession = &recordingSession{
		opts:      opts,
		startTime: time.Now(),
		stats:     startStatsPoller(),
	}
	hooks.PublishWithSource(hooks.EventRecordStart, RecordStartPayload{
		Format:      opts.Format,
		BPM:         opts.BPM,
		Instruments: len(opts.Instruments),
	}, hooks.CaptureSource(0))
	log.Debugf("[record] started (wasm): format=%s instruments=%d",
		opts.Format, len(opts.Instruments))
	return nil
}

// Returns within microseconds: encoding takes 100-500 ms, and blocking the JS-driven Update for that would stutter the
// UI. The download fires when the worker emits the Blob, followed by EventRecordStop.
func StopRecording() (*RecordingResult, error) {
	recordingMu.Lock()
	if activeSession == nil {
		recordingMu.Unlock()
		return nil, fmt.Errorf("no recording in progress")
	}
	session := activeSession
	activeSession = nil
	if session.stats != nil {
		session.stats.stop()
	}
	recordingMu.Unlock()

	duration := time.Since(session.startTime).Seconds()
	timestamp := session.startTime.Format("2006-01-02_150405")
	expectedZip := "beatmo-recording-" + timestamp + ".zip"

	result := &RecordingResult{
		Metadata: SessionMetadata{
			BPM:        session.opts.BPM,
			Duration:   duration,
			SampleRate: SampleRate(),
			Format:     session.opts.Format,
			Timestamp:  timestamp,
		},
		// So the inline toast can show the expected name at once; the async path overwrites it if the worker picks another.
		SessionDir: expectedZip,
	}

	// On the lifecycle pool like desktop, so panics are recovered and goroutines bounded by the registry budget.
	finalizeBegin()

	err := lifecyclePool().Submit(func(_ context.Context) {
		finalizeRecordingAsync(session, result, duration, timestamp)
	})
	if err != nil {
		// Run inline rather than lose the recording.
		log.Warnf("[record] lifecycle pool busy, finalizing sync: %v", err)
		finalizeRecordingAsync(session, result, duration, timestamp)
	}
	return result, nil
}

// The caller does finalizeBegin, so WaitRecordingFinalized works as on desktop.
func finalizeRecordingAsync(session *recordingSession, result *RecordingResult, duration float64, timestamp string) {
	defer finalizeDone()

	finRes, err := platformFinalizeCapture(map[string]any{
		"bpm":            session.opts.BPM,
		"timestamp":      timestamp,
		"duration":       duration,
		"filenamePrefix": "beatmo-recording-",
	})
	if err != nil {
		log.Errorf("[record] finalize failed: %v", err)
		hooks.PublishWithSource(hooks.EventRecordStop, RecordStopPayload{
			Duration: duration,
			Err:      err,
		}, hooks.CaptureSource(0))
		return
	}

	if finRes.SampleRate > 0 {
		result.Metadata.SampleRate = finRes.SampleRate
	}

	// Resolve channel display names from the original instrument list.
	nameByID := make(map[string]string, len(session.opts.Instruments)+1)
	for _, inst := range session.opts.Instruments {
		nameByID[inst.ID.String()] = inst.Name
	}
	nameByID[RecordingMasterID] = "Master"

	for _, ch := range finRes.Channels {
		name := ch.Name
		if name == "" {
			if n, ok := nameByID[ch.ID]; ok {
				name = n
			} else {
				name = ch.ID
			}
		}
		result.Channels = append(result.Channels, EncodedChannel{
			ID:       ch.ID,
			Name:     name,
			Filename: ch.Filename,
			Bytes:    ch.Bytes,
			// The bytes live in the JS Blob; use Bytes for size.
		})
		result.Metadata.Channels = append(result.Metadata.Channels, ChannelMeta{
			ID:       ch.ID,
			Name:     name,
			Filename: ch.Filename,
			Samples:  ch.Samples,
		})
	}

	pendingDownload.Store(&recordingDownloadHandle{
		BlobURL:  finRes.BlobURL,
		Filename: finRes.Filename,
		Size:     finRes.Size,
	})

	if finRes.AutoStopped {
		log.Debugf("[record] auto-stopped: reason=%s", finRes.AutoStopRsn)
	}
	if finRes.Stats.DroppedSamples > 0 {
		hooks.PublishWithSource(hooks.EventRecordDropped, finRes.Stats.DroppedSamples, hooks.CaptureSource(0))
	}

	zipName, saveErr := SaveRecording(result)
	if saveErr != nil {
		log.Errorf("[record] auto-save failed: %v", saveErr)
		hooks.PublishWithSource(hooks.EventRecordStop, RecordStopPayload{
			Dir:      result.SessionDir,
			Duration: duration,
			Channels: len(result.Channels),
			Err:      saveErr,
		}, hooks.CaptureSource(0))
		return
	}
	result.SessionDir = zipName

	log.Debugf("[record] stopped (wasm): duration=%.2fs channels=%d size=%d zip=%s",
		duration, len(result.Channels), finRes.Size, result.SessionDir)
	hooks.PublishWithSource(hooks.EventRecordStop, RecordStopPayload{
		Dir:      result.SessionDir,
		Drops:    finRes.Stats.DroppedSamples,
		Duration: duration,
		Channels: len(result.Channels),
	}, hooks.CaptureSource(0))
}

// IsRecording reports whether a session is active.
func IsRecording() bool {
	recordingMu.Lock()
	defer recordingMu.Unlock()
	return activeSession != nil
}

// RecordingElapsed returns time since the active session started.
func RecordingElapsed() time.Duration {
	recordingMu.Lock()
	defer recordingMu.Unlock()
	if activeSession == nil {
		return 0
	}
	return time.Since(activeSession.startTime)
}

// Max bytes per channel or max duration. Non-blocking.
func RecordingAutoStopped() bool {
	return platformRecordingStatsSnapshot().AutoStopped
}
