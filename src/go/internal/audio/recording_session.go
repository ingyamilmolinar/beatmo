package audio

import (
	"encoding/json"
	"sync"
	"time"
)

// RecordingOptions configures a recording session.
type RecordingOptions struct {
	Format      AudioFormat      // output encoding format (default: FormatWAV24)
	MaxDuration time.Duration    // 0 = unlimited, stop manually
	OutputDir   string           // desktop: filesystem path; WASM: ignored
	BPM         int              // BPM at recording start
	Instruments []InstrumentMeta // snapshot of active instruments
}

// EncodedChannel holds the encoded audio data for one channel.
//
// In streaming mode (desktop production), Data is empty and Path holds
// the absolute path of the WAV file already written to disk during
// recording. In legacy/WASM in-memory mode, Data holds the encoded bytes
// and Path is empty.
type EncodedChannel struct {
	ID       string // instrument ID or "master"
	Name     string // human-readable name
	Filename string // e.g., "kick-deep.wav"
	Data     []byte // encoded audio bytes (nil in streaming mode and in WASM worker mode)
	Path     string // absolute path on disk (empty in in-memory mode)
	// Bytes reports the encoded payload size when Data is intentionally
	// nil (streaming or worker modes). Always populated; use this in
	// preference to len(Data) when reporting size.
	Bytes int
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
	// SessionDir is the on-disk directory holding streamed WAV files
	// (desktop streaming mode). Empty in in-memory mode.
	SessionDir string
}

// recordingMu serializes Start/Stop transitions and protects activeSession.
var recordingMu sync.Mutex

// platformRecordingStart is called when recording begins. On WASM, this
// activates the JS per-channel output capture nodes. On desktop, it is
// a no-op (the streaming pipeline handles capture).
var platformRecordingStart = func(instruments []InstrumentMeta) {}

// platformRecordingStop is called when recording stops. On WASM, this
// retrieves captured samples from the JS per-channel capture nodes.
// Returns nil, nil, 0 when no platform capture is available.
var platformRecordingStop = func() (map[string][]float64, []float64, int) { return nil, nil, 0 }

// MetadataJSON returns the session metadata as formatted JSON bytes.
func (r *RecordingResult) MetadataJSON() ([]byte, error) {
	return json.MarshalIndent(r.Metadata, "", "  ")
}

// InstrumentMeta holds metadata for an instrument being recorded.
type InstrumentMeta struct {
	ID   string
	Name string
}

// RecordStartPayload is the typed payload for hooks.EventRecordStart.
type RecordStartPayload struct {
	Dir         string
	Format      AudioFormat
	BPM         int
	Instruments int
}

// RecordStopPayload is the typed payload for hooks.EventRecordStop. It
// is delivered after the asynchronous finalize (file flush + close)
// completes — at that point the on-disk WAV files are valid and
// readable. Subscribers wanting to act on a recording (toast, upload,
// post-processing) should listen on this event, not on the return value
// of StopRecording (which fires before file flush completes).
type RecordStopPayload struct {
	Dir      string
	Drops    int64
	Duration float64
	Channels int
	Err      error
}
