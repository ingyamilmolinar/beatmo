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
}

// ChannelMetrics holds computed analysis for a single channel.
type ChannelMetrics struct {
	ID        string
	Name      string
	PeakDB    float64
	RMSDB     float64
	ClipCount int
	Waveform  []float64 // rolling window
	FFTBins   []float64 // magnitude dB (nil if not computed)
	FreqBins  []float64 // Hz labels (nil if not computed)
	Envelope  []float64 // amplitude envelope (nil if not computed)
	Active    bool
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

// Config configures the analyzer service.
type Config struct {
	FFTSize        int // power of 2 (default 1024)
	WindowSize     int // analysis window in samples (default 2048)
	MaxInstruments int // max instrument slots (default 32)
	SampleRate     int // audio sample rate (default 44100)
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
