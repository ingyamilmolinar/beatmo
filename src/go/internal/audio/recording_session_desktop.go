//go:build !js

package audio

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// activeSession holds the running recording session, if any.
var activeSession *recordingSession

type recordingSession struct {
	opts      RecordingOptions
	pipe      *pipeline
	startTime time.Time
	dir       string
}

// StartRecording begins a streaming, multi-channel recording session.
// The desktop pipeline writes per-channel WAV files to disk on background
// workers; the audio thread only does a non-blocking handoff.
func StartRecording(opts RecordingOptions) error {
	recordingMu.Lock()
	defer recordingMu.Unlock()

	if activeSession != nil {
		return fmt.Errorf("recording already in progress")
	}
	if opts.Format == "" {
		opts.Format = FormatWAV24
	}
	// Validate the encoder exists even though streaming uses streamWavWriter
	// directly — keeps Start failing fast on a typo.
	if _, err := NewEncoder(opts.Format); err != nil {
		return fmt.Errorf("cannot start recording: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02_150405")
	baseDir := opts.OutputDir
	if baseDir == "" {
		baseDir = recordingsBaseDir()
	}
	dir := filepath.Join(baseDir, timestamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create recording dir: %w", err)
	}

	sr := SampleRate()
	pipe, err := newPipeline(pipelineConfig{
		OutputDir:   dir,
		SampleRate:  sr,
		Format:      opts.Format,
		Instruments: opts.Instruments,
		MaxDuration: opts.MaxDuration,
	})
	if err != nil {
		return fmt.Errorf("init pipeline: %w", err)
	}

	activeSession = &recordingSession{
		opts:      opts,
		pipe:      pipe,
		startTime: time.Now(),
		dir:       dir,
	}
	pipelinePtr.Store(pipe)
	platformRecordingStart(opts.Instruments)

	log.Printf("[RECORDING] Started: format=%s instruments=%d dir=%s maxDuration=%v",
		opts.Format, len(opts.Instruments), dir, opts.MaxDuration)
	hooks.PublishKind(hooks.EventRecordStart, dir)
	return nil
}

// StopRecording detaches the pipeline from the audio thread (fast,
// in-place) and queues the slow drain + close + save work onto the
// recording.lifecycle pool. Returns immediately with a RecordingResult
// whose Channels carry per-channel file paths. The Sample counts in
// Metadata.Channels remain placeholder zero until the async finalize
// completes; subscribers that need finalized stats should listen on
// hooks.EventRecordStop.
//
// UI thread cost: detach (microseconds, WaitGroup-based barrier) +
// snapshot of channel paths. No file I/O on the calling goroutine.
//
// Tests that need to read the WAV files synchronously should call
// audio.WaitRecordingFinalized(ctx) to block until the lifecycle worker
// flushes and closes everything.
func StopRecording() (*RecordingResult, error) {
	recordingMu.Lock()
	defer recordingMu.Unlock()

	if activeSession == nil {
		return nil, fmt.Errorf("no recording in progress")
	}

	session := activeSession
	activeSession = nil

	// Detach hot path FIRST so no new Taps land on the pipeline. The
	// WaitGroup barrier in Detach guarantees no Tap goroutine is mid-
	// execution when this returns.
	pipelinePtr.Store(nil)
	if err := session.pipe.Detach(); err != nil {
		log.Printf("[RECORDING] Detach pipeline: %v", err)
	}
	// DrainWorkers is fast (close channels + wait for workers to flush
	// the in-flight queue into bufio buffers). Doing this synchronously
	// here means callers see accurate sample counts in the returned
	// Result, while the slow file Close runs async via queueFinalize.
	if err := session.pipe.DrainWorkers(); err != nil {
		log.Printf("[RECORDING] DrainWorkers: %v", err)
	}

	// Allow platform (e.g., test stub) to publish anything it has
	// captured. Desktop production no-ops here.
	platChannels, platMaster, platSR := platformRecordingStop()
	sr := SampleRate()

	duration := time.Since(session.startTime).Seconds()
	pipeChannels := session.pipe.Channels()
	result := &RecordingResult{
		Channels:   pipeChannels,
		SessionDir: session.dir,
		Metadata: SessionMetadata{
			BPM:        session.opts.BPM,
			Duration:   duration,
			SampleRate: sr,
			Format:     session.opts.Format,
			Timestamp:  session.startTime.Format("2006-01-02_150405"),
			// MetaSnapshot reads atomic counts populated by workers
			// during DrainWorkers — accurate by the time we get here.
			Channels: session.pipe.MetaSnapshot(),
		},
	}

	if len(pipeChannels) == 0 && (len(platMaster) > 0 || len(platChannels) > 0) {
		// In-memory platform fallback (legacy WASM-style stubs in tests).
		if platSR > 0 {
			result.Metadata.SampleRate = platSR
		}
		result.Channels = encodeInMemoryFallback(session.opts, platChannels, platMaster, result.Metadata.SampleRate, &result.Metadata)
		// In-memory path has no async tail; close synchronously to keep
		// the pipeline goroutines from leaking, then return.
		_, _ = session.pipe.CloseWriters()
		log.Printf("[RECORDING] Stopped (in-memory fallback): duration=%.2fs channels=%d",
			duration, len(result.Channels))
		hooks.PublishKind(hooks.EventRecordStop, RecordStopPayload{
			Dir: session.dir, Duration: duration, Channels: len(result.Channels),
		})
		return result, nil
	}

	if drops := session.pipe.Drops(); drops > 0 {
		hooks.PublishKind(hooks.EventRecordDropped, drops)
	}
	log.Printf("[RECORDING] Detached: duration=%.2fs channels=%d drops=%d dir=%s — finalizing async",
		duration, len(result.Channels), session.pipe.Drops(), session.dir)

	// Queue the slow drain+close+save tail onto the lifecycle pool. The
	// final EventRecordStop with finalized stats fires from there.
	queueFinalize(session, result)
	return result, nil
}

// IsRecording reports whether a session is active.
func IsRecording() bool {
	return pipelinePtr.Load() != nil
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

// RecordingAutoStopped reports whether the session reached its max-duration cap.
func RecordingAutoStopped() bool {
	p := pipelinePtr.Load()
	if p == nil {
		return false
	}
	return p.IsAutoStopped()
}

// encodeInMemoryFallback handles platform-provided samples (e.g., the
// recording_functional_test.go stubs that simulate the WASM platform path)
// by encoding them into in-memory EncodedChannel.Data. Only invoked when
// the streaming pipeline produced no output.
func encodeInMemoryFallback(
	opts RecordingOptions,
	platChannels map[string][]float64,
	platMaster []float64,
	sr int,
	meta *SessionMetadata,
) []EncodedChannel {
	encoder, err := NewEncoder(opts.Format)
	if err != nil {
		return nil
	}
	ext := encoder.FileExtension()
	var out []EncodedChannel
	if len(platMaster) > 0 {
		buf, err := encodeBytes(encoder, platMaster, sr)
		if err == nil {
			filename := "master" + ext
			out = append(out, EncodedChannel{ID: "master", Name: "Master", Filename: filename, Data: buf})
			meta.Channels = append(meta.Channels, ChannelMeta{ID: "master", Name: "Master", Filename: filename, Samples: len(platMaster)})
		}
	}
	for _, inst := range opts.Instruments {
		samples, ok := platChannels[inst.ID]
		if !ok || len(samples) == 0 {
			continue
		}
		buf, err := encodeBytes(encoder, samples, sr)
		if err != nil {
			log.Printf("[RECORDING] encode %s: %v", inst.ID, err)
			continue
		}
		filename := inst.ID + ext
		out = append(out, EncodedChannel{ID: inst.ID, Name: inst.Name, Filename: filename, Data: buf})
		meta.Channels = append(meta.Channels, ChannelMeta{ID: inst.ID, Name: inst.Name, Filename: filename, Samples: len(samples)})
	}
	return out
}
