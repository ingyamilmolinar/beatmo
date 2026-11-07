//go:build !js

package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// activeSession holds the running recording session, if any.
var activeSession *recordingSession

type recordingSession struct {
	opts      RecordingOptions
	pipe      *pipeline
	startTime time.Time
	dir       string
}

// The audio thread only hands off; workers write per-channel WAVs.
func StartRecording(opts RecordingOptions) error {
	recordingMu.Lock()
	defer recordingMu.Unlock()

	if activeSession != nil {
		return fmt.Errorf("recording already in progress")
	}
	if opts.Format == "" {
		opts.Format = FormatWAV24
	}
	// Fails fast on a format typo, though streaming bypasses the encoder.
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

	log.Debugf("[record] started: format=%s instruments=%d dir=%s maxDuration=%v",
		opts.Format, len(opts.Instruments), dir, opts.MaxDuration)
	hooks.PublishWithSource(hooks.EventRecordStart, RecordStartPayload{
		Dir:         dir,
		Format:      opts.Format,
		BPM:         opts.BPM,
		Instruments: len(opts.Instruments),
	}, hooks.CaptureSource(0))
	return nil
}

// Returns at once with each channel's path; the slow drain + close + save runs on the lifecycle pool, and finalized
// stats arrive on EventRecordStop. Tests reading the files call WaitRecordingFinalized.
func StopRecording() (*RecordingResult, error) {
	recordingMu.Lock()
	defer recordingMu.Unlock()

	if activeSession == nil {
		return nil, fmt.Errorf("no recording in progress")
	}

	session := activeSession
	activeSession = nil

	// FIRST, so no new Tap lands on the pipeline.
	pipelinePtr.Store(nil)
	if err := session.pipe.Detach(); err != nil {
		log.Errorf("[record] detach pipeline: %v", err)
	}
	// Synchronous so the result carries accurate counts; the slow Close runs async.
	if err := session.pipe.DrainWorkers(); err != nil {
		log.Errorf("[record] DrainWorkers: %v", err)
	}

	// A no-op in desktop production; test stubs publish here.
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
			Channels:   session.pipe.MetaSnapshot(),
		},
	}

	if len(pipeChannels) == 0 && (len(platMaster) > 0 || len(platChannels) > 0) {
		// In-memory platform fallback (legacy WASM-style stubs in tests).
		if platSR > 0 {
			result.Metadata.SampleRate = platSR
		}
		result.Channels = encodeInMemoryFallback(session.opts, platChannels, platMaster, result.Metadata.SampleRate, &result.Metadata)
		// No async tail: close now so the pipeline goroutines do not leak.
		_, _ = session.pipe.CloseWriters()
		log.Warnf("[record] stopped (in-memory fallback): duration=%.2fs channels=%d",
			duration, len(result.Channels))
		hooks.PublishWithSource(hooks.EventRecordStop, RecordStopPayload{
			Dir: session.dir, Duration: duration, Channels: len(result.Channels),
		}, hooks.CaptureSource(0))
		return result, nil
	}

	if drops := session.pipe.Drops(); drops > 0 {
		hooks.PublishWithSource(hooks.EventRecordDropped, drops, hooks.CaptureSource(0))
	}
	log.Debugf("[record] detached: duration=%.2fs channels=%d drops=%d dir=%s — finalizing async",
		duration, len(result.Channels), session.pipe.Drops(), session.dir)

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

// Only when the streaming pipeline produced nothing (the stubs that simulate the WASM path).
func encodeInMemoryFallback(
	opts RecordingOptions,
	platChannels map[channel.ID][]float64,
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
			filename := RecordingMasterID + ext
			out = append(out, EncodedChannel{ID: RecordingMasterID, Name: "Master", Filename: filename, Data: buf})
			meta.Channels = append(meta.Channels, ChannelMeta{ID: RecordingMasterID, Name: "Master", Filename: filename, Samples: len(platMaster)})
		}
	}
	for _, inst := range opts.Instruments {
		samples, ok := platChannels[inst.ID]
		if !ok || len(samples) == 0 {
			continue
		}
		buf, err := encodeBytes(encoder, samples, sr)
		if err != nil {
			log.Errorf("[record] encode %s: %v", inst.ID, err)
			continue
		}
		filename := inst.ID.String() + ext
		out = append(out, EncodedChannel{ID: inst.ID.String(), Name: inst.Name, Filename: filename, Data: buf})
		meta.Channels = append(meta.Channels, ChannelMeta{ID: inst.ID.String(), Name: inst.Name, Filename: filename, Samples: len(samples)})
	}
	return out
}
