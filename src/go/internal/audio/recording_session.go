package audio

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// RecordingOptions configures a recording session.
type RecordingOptions struct {
	Format      AudioFormat      // output encoding format (default: FormatWAV24)
	MaxDuration time.Duration    // 0 = unlimited, stop manually
	OutputDir   string           // desktop: filesystem path; WASM: ignored
	BPM         int              // BPM at recording start
	Instruments []InstrumentMeta // snapshot of active instruments
}

// The master-bus stem ("master.<ext>"); instrument stems are named by their channel.
const RecordingMasterID = "master"

type EncodedChannel struct {
	ID       string // the instrument channel recorded, or RecordingMasterID
	Name     string // human-readable name
	Filename string // e.g., "kick-deep.wav"
	Data     []byte // encoded audio bytes (nil in streaming mode and in WASM worker mode)
	Path     string // absolute path on disk (empty in in-memory mode)
	// Data is nil in streaming and worker modes: report size from this.
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
	// Empty in in-memory mode.
	SessionDir string
}

// recordingMu serializes Start/Stop transitions and protects activeSession.
var recordingMu sync.Mutex

// On WASM it activates the JS capture nodes; desktop's streaming pipeline needs nothing.
var platformRecordingStart = func(instruments []InstrumentMeta) {}

// nil, nil, 0 when the platform captured nothing.
var platformRecordingStop = func() (map[channel.ID][]float64, []float64, int) { return nil, nil, 0 }

// MetadataJSON returns the session metadata as formatted JSON bytes.
func (r *RecordingResult) MetadataJSON() ([]byte, error) {
	return json.MarshalIndent(r.Metadata, "", "  ")
}

// InstrumentMeta holds metadata for an instrument being recorded.
type InstrumentMeta struct {
	ID   channel.ID
	Name string
}

// RecordStartPayload is the typed payload for hooks.EventRecordStart.
type RecordStartPayload struct {
	Dir         string
	Format      AudioFormat
	BPM         int
	Instruments int
}

// Delivered after the async finalize, when the files are valid: act on a recording here, not on StopRecording's return.
type RecordStopPayload struct {
	Dir      string
	Drops    int64
	Duration float64
	Channels int
	Err      error
}
