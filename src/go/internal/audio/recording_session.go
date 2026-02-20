package audio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// RecordingOptions configures a recording session.
type RecordingOptions struct {
	Format      AudioFormat       // output encoding format (default: FormatWAV24)
	MaxDuration time.Duration     // 0 = unlimited, stop manually
	OutputDir   string            // desktop: filesystem path; WASM: ignored
	BPM         int               // BPM at recording start
	Instruments []InstrumentMeta  // snapshot of active instruments
}

// EncodedChannel holds the encoded audio data for one channel.
type EncodedChannel struct {
	ID       string // instrument ID or "master"
	Name     string // human-readable name
	Filename string // e.g., "kick-deep.wav"
	Data     []byte // encoded audio data
}

// SessionMetadata holds recording session information.
type SessionMetadata struct {
	BPM        int           `json:"bpm"`
	Duration   float64       `json:"duration_seconds"`
	SampleRate int           `json:"sample_rate"`
	Format     AudioFormat   `json:"format"`
	Timestamp  string        `json:"timestamp"`
	Channels   []ChannelMeta `json:"channels"`
}

// ChannelMeta describes one channel in the recording metadata.
type ChannelMeta struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Filename string `json:"filename"`
	Samples  int    `json:"samples"`
}

// RecordingResult contains the output of a completed recording session.
type RecordingResult struct {
	Channels []EncodedChannel
	Metadata SessionMetadata
}

var (
	recordingMu      sync.Mutex
	activeSession    *recordingSession
)

// platformRecordingStart is called when recording begins. On WASM, this
// activates the JS per-channel output capture nodes. Receives the list
// of instruments to capture. On desktop, it's a no-op (the Go mixer's
// appendBlock handles capture).
var platformRecordingStart = func(instruments []InstrumentMeta) {}

// platformRecordingStop is called when recording stops. On WASM, this
// retrieves captured samples from the JS output capture nodes. Returns
// per-instrument channels (map[id]samples), master samples, and the
// sample rate. Returns nil, nil, 0 if no platform capture is available.
var platformRecordingStop = func() (map[string][]float64, []float64, int) { return nil, nil, 0 }

type recordingSession struct {
	opts      RecordingOptions
	capture   *MultiChannelCapture
	startTime time.Time
}

// StartRecording begins a multi-channel recording session.
// It creates capture channels for all specified instruments plus master,
// and immediately starts capturing audio from the mixer.
func StartRecording(opts RecordingOptions) error {
	recordingMu.Lock()
	defer recordingMu.Unlock()

	if activeSession != nil {
		return fmt.Errorf("recording already in progress")
	}

	if opts.Format == "" {
		opts.Format = FormatWAV24
	}

	// Validate encoder availability
	if _, err := NewEncoder(opts.Format); err != nil {
		return fmt.Errorf("cannot start recording: %w", err)
	}

	sr := SampleRate()
	mc := newMultiChannelCapture(opts.Instruments, sr, opts.MaxDuration)

	activeSession = &recordingSession{
		opts:      opts,
		capture:   mc,
		startTime: time.Now(),
	}

	// Activate capture on the hot path (atomic store)
	multiCapturePtr.Store(mc)

	// Activate platform-specific capture (e.g., JS per-channel capture on WASM)
	platformRecordingStart(opts.Instruments)

	log.Printf("[RECORDING] Started: format=%s instruments=%d maxDuration=%v",
		opts.Format, len(opts.Instruments), opts.MaxDuration)
	return nil
}

// StopRecording stops the active recording session, encodes all channels,
// and returns the result. The encoding happens synchronously.
func StopRecording() (*RecordingResult, error) {
	recordingMu.Lock()
	defer recordingMu.Unlock()

	if activeSession == nil {
		return nil, fmt.Errorf("no recording in progress")
	}

	// Deactivate capture on the hot path
	multiCapturePtr.Store(nil)

	// Retrieve platform-specific capture (e.g., JS per-channel capture on WASM)
	platChannels, platMaster, platSR := platformRecordingStop()

	session := activeSession
	activeSession = nil

	// Get captured data
	channels, master := session.capture.snapshot()
	duration := time.Since(session.startTime).Seconds()

	// If Go mixer didn't capture audio (WASM), use platform capture
	sr := SampleRate()
	if len(master.Samples) == 0 && len(platMaster) > 0 {
		master.Samples = platMaster
		if platSR > 0 {
			sr = platSR
		}
		// Inject per-instrument channels from platform capture
		for id, samples := range platChannels {
			if len(samples) == 0 {
				continue
			}
			// Find the instrument name from recording options
			name := id
			for _, inst := range session.opts.Instruments {
				if inst.ID == id {
					name = inst.Name
					break
				}
			}
			channels[id] = &CaptureChannel{
				ID:      id,
				Name:    name,
				Samples: samples,
			}
		}
		log.Printf("[RECORDING] Using platform capture: %d master samples, %d instrument channels",
			len(platMaster), len(platChannels))
	}

	log.Printf("[RECORDING] Stopped: duration=%.2fs channels=%d masterSamples=%d",
		duration, len(channels), len(master.Samples))

	// Encode all channels
	encoder, err := NewEncoder(session.opts.Format)
	if err != nil {
		return nil, fmt.Errorf("encoder error: %w", err)
	}

	ext := encoder.FileExtension()

	result := &RecordingResult{
		Metadata: SessionMetadata{
			BPM:        session.opts.BPM,
			Duration:   duration,
			SampleRate: sr,
			Format:     session.opts.Format,
			Timestamp:  session.startTime.Format("2006-01-02_150405"),
		},
	}

	// Encode master channel
	if len(master.Samples) > 0 {
		var buf bytes.Buffer
		if err := encoder.Encode(&buf, master.Samples, sr); err != nil {
			return nil, fmt.Errorf("failed to encode master: %w", err)
		}
		filename := "master" + ext
		result.Channels = append(result.Channels, EncodedChannel{
			ID:       "master",
			Name:     "Master",
			Filename: filename,
			Data:     buf.Bytes(),
		})
		result.Metadata.Channels = append(result.Metadata.Channels, ChannelMeta{
			ID: "master", Name: "Master", Filename: filename, Samples: len(master.Samples),
		})
	}

	// Encode per-instrument channels
	for _, inst := range session.opts.Instruments {
		ch, ok := channels[inst.ID]
		if !ok || len(ch.Samples) == 0 {
			continue
		}
		var buf bytes.Buffer
		if err := encoder.Encode(&buf, ch.Samples, sr); err != nil {
			log.Printf("[RECORDING] Warning: failed to encode %s: %v", inst.ID, err)
			continue
		}
		filename := inst.ID + ext
		result.Channels = append(result.Channels, EncodedChannel{
			ID:       inst.ID,
			Name:     inst.Name,
			Filename: filename,
			Data:     buf.Bytes(),
		})
		result.Metadata.Channels = append(result.Metadata.Channels, ChannelMeta{
			ID: inst.ID, Name: inst.Name, Filename: filename, Samples: len(ch.Samples),
		})
	}

	return result, nil
}

// IsRecording returns whether a recording session is active.
func IsRecording() bool {
	return multiCapturePtr.Load() != nil
}

// RecordingElapsed returns the duration since recording started.
// Returns 0 if not recording.
func RecordingElapsed() time.Duration {
	recordingMu.Lock()
	defer recordingMu.Unlock()
	if activeSession == nil {
		return 0
	}
	return time.Since(activeSession.startTime)
}

// RecordingAutoStopped returns true if the recording reached its max duration.
func RecordingAutoStopped() bool {
	mc := multiCapturePtr.Load()
	if mc == nil {
		return false
	}
	return mc.isDone()
}

// MetadataJSON returns the session metadata as formatted JSON bytes.
func (r *RecordingResult) MetadataJSON() ([]byte, error) {
	return json.MarshalIndent(r.Metadata, "", "  ")
}
