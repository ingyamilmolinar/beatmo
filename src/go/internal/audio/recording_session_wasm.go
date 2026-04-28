//go:build js && wasm

package audio

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// activeSession holds the running WASM recording session, if any.
var activeSession *recordingSession

type recordingSession struct {
	opts      RecordingOptions
	startTime time.Time
	stats     *statsPoller
}

// StartRecording activates the off-thread capture+encode pipeline. The
// browser-side worklet (audio thread) feeds samples directly into a Web
// Worker (encoder thread); the Go-WASM goroutine that calls into here
// only signals "go" and immediately returns. No audio data ever crosses
// the main thread on the hot path.
//
// Briefly blocks (typically <50 ms) waiting for the encoder Worker to
// confirm readiness — without this barrier the first audio frames could
// arrive at the worker before the channel ports are wired and would be
// silently dropped. The hitch is below one frame at 60 fps and matches
// the cost of any user gesture that crosses the JS/WASM boundary once.
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
	hooks.PublishKind(hooks.EventRecordStart, RecordStartPayload{
		Format:      opts.Format,
		BPM:         opts.BPM,
		Instruments: len(opts.Instruments),
	})
	log.Printf("[RECORDING/wasm] Started: format=%s instruments=%d",
		opts.Format, len(opts.Instruments))
	return nil
}

// StopRecording immediately marks the session as stopping, then dispatches
// the slow worker-await + download trigger onto a background goroutine so
// the calling Update tick returns within microseconds. The returned
// RecordingResult holds metadata + an "expected" SessionDir filename for
// the inline UI toast; the actual download fires when the worker emits
// the Blob, and a subsequent EventRecordStop hook delivers the final
// "Recording saved" toast (matching desktop UX).
//
// Why async: the worker can take 100-500 ms to encode and bundle a
// multi-channel session. Blocking the JS-driven Update tick for that
// long would stutter the UI and force the game loop to skip RAF ticks.
// Spawning a goroutine instead lets the Go scheduler yield to JS while
// awaitJSPromise blocks on the encoder Worker, keeping the UI fluid.
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
		// Pre-populate so the UI's inline "Saving recording to ..." toast
		// can render the expected filename immediately. The async path
		// will overwrite if the worker assigns a different name.
		SessionDir: expectedZip,
	}

	// Background finalize on the canonical recording.lifecycle pool: the
	// Go-WASM scheduler yields to the JS event loop while awaitJSPromise
	// blocks, so neither the audio thread nor the UI thread is impacted.
	// Mirrors the desktop queueFinalize pattern (recording_lifecycle.go)
	// so panics are recovered by the pool worker and the goroutine count
	// is bounded by the registry budget.
	finalizeMu.Lock()
	finalizePending.Add(1)
	finalizeMu.Unlock()

	err := lifecyclePool().Submit(func(_ context.Context) {
		finalizeRecordingAsync(session, result, duration, timestamp)
	})
	if err != nil {
		// Pool saturated or closed — run inline rather than lose the
		// recording. activeSession is already nil so the caller has
		// already returned; this just makes Stop synchronous in the
		// degenerate case.
		log.Printf("[RECORDING/wasm] lifecycle pool busy, finalizing sync: %v", err)
		finalizeRecordingAsync(session, result, duration, timestamp)
	}
	return result, nil
}

// finalizeRecordingAsync drives the JS encoder Worker through finalize,
// triggers the browser download, and publishes EventRecordStop. Runs on
// the recording.lifecycle pool worker (or inline as a fallback); never
// blocks the UI thread. The matching finalizePending.Add(1) is performed
// by the caller (StopRecording) so WaitRecordingFinalized is symmetric
// with the desktop path.
func finalizeRecordingAsync(session *recordingSession, result *RecordingResult, duration float64, timestamp string) {
	defer finalizePending.Done()

	finRes, err := platformFinalizeCapture(map[string]any{
		"bpm":            session.opts.BPM,
		"timestamp":      timestamp,
		"duration":       duration,
		"filenamePrefix": "beatmo-recording-",
	})
	if err != nil {
		log.Printf("[RECORDING/wasm] finalize failed: %v", err)
		hooks.PublishKind(hooks.EventRecordStop, RecordStopPayload{
			Duration: duration,
			Err:      err,
		})
		return
	}

	if finRes.SampleRate > 0 {
		result.Metadata.SampleRate = finRes.SampleRate
	}

	// Resolve channel display names from the original instrument list.
	nameByID := make(map[string]string, len(session.opts.Instruments)+1)
	for _, inst := range session.opts.Instruments {
		nameByID[inst.ID] = inst.Name
	}
	nameByID["master"] = "Master"

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
			// Data deliberately nil: the encoded bytes live in the JS Blob
			// referenced by the cached download handle. Use Bytes for size.
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
		log.Printf("[RECORDING/wasm] auto-stopped: reason=%s", finRes.AutoStopRsn)
	}
	if finRes.Stats.DroppedSamples > 0 {
		hooks.PublishKind(hooks.EventRecordDropped, finRes.Stats.DroppedSamples)
	}

	zipName, saveErr := SaveRecording(result)
	if saveErr != nil {
		log.Printf("[RECORDING/wasm] auto-save failed: %v", saveErr)
		hooks.PublishKind(hooks.EventRecordStop, RecordStopPayload{
			Dir:      result.SessionDir,
			Duration: duration,
			Channels: len(result.Channels),
			Err:      saveErr,
		})
		return
	}
	result.SessionDir = zipName

	log.Printf("[RECORDING/wasm] Stopped: duration=%.2fs channels=%d size=%d zip=%s",
		duration, len(result.Channels), finRes.Size, result.SessionDir)
	hooks.PublishKind(hooks.EventRecordStop, RecordStopPayload{
		Dir:      result.SessionDir,
		Drops:    finRes.Stats.DroppedSamples,
		Duration: duration,
		Channels: len(result.Channels),
	})
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

// RecordingAutoStopped reports whether the worker tripped a hard cap
// (max bytes per channel or max duration). Reads the cached worker stats
// — non-blocking.
func RecordingAutoStopped() bool {
	return platformRecordingStatsSnapshot().AutoStopped
}
