package scopeexport

// Snapshot is a single JSONL line capturing all pipeline stages for all
// instruments and master at a point in time.
type Snapshot struct {
	TS         string        `json:"ts"`
	Tick       int64         `json:"tick"`
	ElapsedMs  int64         `json:"elapsed_ms"`
	BPM        int           `json:"bpm"`
	SampleRate int           `json:"sample_rate"`
	Channels   []ChannelSnap `json:"channels"`
	Master     MasterSnap    `json:"master"`
}

// ChannelSnap captures per-instrument pipeline data.
type ChannelSnap struct {
	ID     string                   `json:"id"`
	Name   string                   `json:"name,omitempty"`
	Kind   string                   `json:"kind,omitempty"`
	Volume float64                  `json:"volume"`
	Pan    float64                  `json:"pan"`
	Stages map[string]*StageMetrics `json:"stages"`
}

// MasterSnap captures master bus pipeline data.
type MasterSnap struct {
	Volume float64                  `json:"volume"`
	Stages map[string]*StageMetrics `json:"stages"`
}

// StageMetrics holds computed metrics for a single pipeline stage.
type StageMetrics struct {
	PeakDB        float64      `json:"peak_db"`
	RMSDB         float64      `json:"rms_db"`
	ClipCount     int          `json:"clip_count"`
	Waveform64    [][2]float64 `json:"waveform_64"`
	FFTTop        []FFTBin     `json:"fft_top"`
	ZeroCrossings int          `json:"zero_crossings"`
	ZCRate        float64      `json:"zc_rate"`
}

// FFTBin is a single frequency bin with magnitude.
type FFTBin struct {
	Hz float64 `json:"hz"`
	DB float64 `json:"db"`
}
