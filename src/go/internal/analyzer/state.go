// Package analyzer provides real-time audio analysis types and services
// for metering, FFT spectrum display, and waveform capture.
package analyzer

import (
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

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

// The L/R fields carry stereo when a producer fills them; the mono fields stay the downmixed view, and HasStereo /
// PeakL / PeakR / RMSL / RMSR fall back to them, so renderers use one accessor either way.
type ChannelMetrics struct {
	ID              channel.ID
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

	WaveformL []float64
	WaveformR []float64
	FFTBinsL  []float64
	FFTBinsR  []float64
	PeakDBL   float64
	PeakDBR   float64
	RMSDBL    float64
	RMSDBR    float64
}

// False: use the mono fields directly.
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
	ID        channel.ID
	Name      string
	PeakDB    float64
	RMSDB     float64
	ClipCount int
	Active    bool
}

// CaptureBuffer holds a captured waveform for static inspection.
type CaptureBuffer struct {
	InstID     channel.ID
	Wave       wave.Wave
	Frozen     bool
	TriggerIdx int64
}

// Config configures the analyzer service.
type Config struct {
	FFTSize        int // power of 2 (default SpectrumFFTSize)
	WindowSize     int // analysis window in samples (default 2048)
	MaxInstruments int // max instrument slots (default 32)
	SampleRate     int // audio sample rate (default 44100)

	// Called once per tick into LUFSShortTermDB, so the audio package owns the K-weighting integrator without an import
	// cycle. -120 when silent.
	MasterLUFSGetter func() float64

	// The rolling 10 s clip count, so the Levels CLIPS line never shows an ever-climbing session total.
	ClipsLastWindowGetter func() int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		FFTSize:        SpectrumFFTSize,
		WindowSize:     2048,
		MaxInstruments: 32,
		SampleRate:     44100,
	}
}
