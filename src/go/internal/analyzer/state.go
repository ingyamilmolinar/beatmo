// Package analyzer provides real-time audio analysis types and services
// for metering, FFT spectrum display, and waveform capture.
package analyzer

import "github.com/ingyamilmolinar/beatmo/internal/wave"

// State is the atomically-published snapshot the UI reads.
type State struct {
	Instruments []InstrumentMetrics
	Master      ChannelMetrics
	Detail      *ChannelMetrics // selected instrument (full FFT + envelope)
	Capture     *CaptureBuffer
	Timestamp   int64 // monotonic nanoseconds

	// ClipsLastWindow is the rolling 10-second clip count summed across
	// all instrument channels + master. Set by Service from
	// Config.ClipsLastWindowGetter (audio package owns the window).
	ClipsLastWindow int
}

// ChannelMetrics holds computed analysis for a single channel.
//
// Phase 5 schema: WaveformL/R, FFTBinsL/R, PeakDBL/R, RMSDBL/R carry
// stereo data when the analyzer pipeline produces it. The existing
// mono fields (Waveform, FFTBins, PeakDB, RMSDB) remain the canonical
// "downmixed" view. Renderers can call HasStereo() / PeakL() / PeakR()
// / RMSL() / RMSR() and get a sensible value either way — the helpers
// fall back to the mono field when stereo data isn't present. The data
// path that populates the L/R fields is wired separately (mixer +
// analyzer changes); the schema landing first lets the renderer side
// migrate independently.
type ChannelMetrics struct {
	ID              string
	Name            string
	PeakDB          float64
	RMSDB           float64
	TruePeakDB      float64 // ISP / oversampled peak; equals PeakDB until ISP computation lands
	HeadroomDB      float64 // 0 - PeakDB; negative on clip
	LUFSShortTermDB float64 // ITU-R BS.1770 short-term loudness; 0 until LUFS observer lands
	ClipCount       int
	Waveform        []float64 // rolling window (mono / downmix)
	FFTBins         []float64 // magnitude dB (mono / downmix; nil if not computed)
	FreqBins        []float64 // Hz labels (nil if not computed)
	Envelope        []float64 // amplitude envelope (nil if not computed)
	Active          bool

	// ── Phase 5: stereo extensions ────────────────────────────────────
	WaveformL []float64
	WaveformR []float64
	FFTBinsL  []float64
	FFTBinsR  []float64
	PeakDBL   float64
	PeakDBR   float64
	RMSDBL    float64
	RMSDBR    float64
}

// HasStereo reports whether any of the L/R fields are populated. False
// means the renderer should treat this channel as mono and use the
// canonical Waveform / FFTBins / PeakDB / RMSDB fields directly.
//
// NOTE: as of Phase 5 the analyzer/service compute path and engine mixer
// taps remain mono — this returns false for all real data in production.
// The renderer L/R branches in render_meters.go and render_waveform.go
// are dormant until the upstream wiring lands (see plan §Phase 5.2-5.3).
func (c *ChannelMetrics) HasStereo() bool {
	return len(c.WaveformL) > 0 || len(c.WaveformR) > 0 ||
		len(c.FFTBinsL) > 0 || len(c.FFTBinsR) > 0 ||
		c.PeakDBL != 0 || c.PeakDBR != 0 ||
		c.RMSDBL != 0 || c.RMSDBR != 0
}

// PeakL returns the left-channel peak dB if stereo data is present,
// otherwise the mono PeakDB so callers can use a single accessor.
func (c *ChannelMetrics) PeakL() float64 {
	if c.PeakDBL != 0 {
		return c.PeakDBL
	}
	return c.PeakDB
}

// PeakR returns the right-channel peak dB if stereo data is present,
// otherwise the mono PeakDB.
func (c *ChannelMetrics) PeakR() float64 {
	if c.PeakDBR != 0 {
		return c.PeakDBR
	}
	return c.PeakDB
}

// RMSL returns the left-channel RMS dB if stereo data is present,
// otherwise the mono RMSDB.
func (c *ChannelMetrics) RMSL() float64 {
	if c.RMSDBL != 0 {
		return c.RMSDBL
	}
	return c.RMSDB
}

// RMSR returns the right-channel RMS dB if stereo data is present,
// otherwise the mono RMSDB.
func (c *ChannelMetrics) RMSR() float64 {
	if c.RMSDBR != 0 {
		return c.RMSDBR
	}
	return c.RMSDB
}

// InstrumentMetrics holds lightweight per-instrument data for the meter bridge.
type InstrumentMetrics struct {
	ID        string
	Name      string
	PeakDB    float64
	RMSDB     float64
	ClipCount int
	Active    bool
}

// CaptureBuffer holds a captured waveform for static inspection.
type CaptureBuffer struct {
	InstID     string
	Wave       wave.Wave
	Frozen     bool
	TriggerIdx int64
}

// ClipsLastWindow is the rolling 10-second clip count summed across all
// channels (set by Service from Config.ClipsLastWindowGetter). The UI
// reads this for the LevelsAggregates "CLIPS" line so a long-running
// session doesn't keep showing an ever-climbing monotonic counter.
//
// Access via State.ClipsLastWindow (see below).

// Config configures the analyzer service.
type Config struct {
	FFTSize        int // power of 2 (default 1024)
	WindowSize     int // analysis window in samples (default 2048)
	MaxInstruments int // max instrument slots (default 32)
	SampleRate     int // audio sample rate (default 44100)

	// MasterLUFSGetter, when non-nil, is called once per processTick and
	// its return value is written into State.Master.LUFSShortTermDB.
	// Lets the audio package own the K-weighting filter + 3 s
	// integrator (audio/loudness.go) without creating a circular
	// dependency on this package. Returns -120 when integrator is
	// empty / silent.
	MasterLUFSGetter func() float64

	// ClipsLastWindowGetter, when non-nil, returns the rolling 10 s
	// clip count (sum of all per-channel clips that occurred in the
	// last sliding window). Wired through Service.State() so the UI's
	// LevelsAggregates side panel can display "Clips last 10 s" instead
	// of a session-total monotonic counter.
	ClipsLastWindowGetter func() int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		FFTSize:        1024,
		WindowSize:     2048,
		MaxInstruments: 32,
		SampleRate:     44100,
	}
}
