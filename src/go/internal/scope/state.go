// Package scope provides real-time oscilloscope data types and services for A/B pipeline stage comparison.
package scope

import "github.com/ingyamilmolinar/beatmo/internal/channel"

// Stage represents a named point in the audio signal chain.
type Stage int

const (
	StageSynth Stage = iota
	StageAntiPop
	StageInsertFX
	StageEQ
	StageMaster
	stageCount
)

// Short enough for the Chain tab's compact stage buttons; the long form is StageDescription.
func StageLabel(s Stage) string {
	switch s {
	case StageSynth:
		return "Synth"
	case StageAntiPop:
		return "AntiPop"
	case StageInsertFX:
		return "FX"
	case StageEQ:
		return "EQ"
	case StageMaster:
		return "Master"
	default:
		return ""
	}
}

// For tooltips and the Chain tab's plain-English explanations; "" for unknown stages.
func StageDescription(s Stage) string {
	switch s {
	case StageSynth:
		return "Synth output (raw note before any effect)"
	case StageAntiPop:
		return "Click guard (smooths note start/stop)"
	case StageInsertFX:
		return "Insert FX chain (distortion, delay, reverb, …)"
	case StageEQ:
		return "EQ (per-band bass/treble shaping)"
	case StageMaster:
		return "Master (post-mix, post-compressor)"
	default:
		return ""
	}
}

// NumStages is how many stages the pipeline has; arrays indexed by Stage use it.
const NumStages = int(stageCount)

// AllStages returns all stages in signal-flow order.
func AllStages() []Stage {
	stages := make([]Stage, stageCount)
	for i := 0; i < int(stageCount); i++ {
		stages[i] = Stage(i)
	}
	return stages
}

// State holds a captured oscilloscope snapshot with two tap points for A/B comparison.
type State struct {
	TapA      TapData
	TapB      TapData
	Timestamp int64 // monotonic nanoseconds
}

// TapData contains captured audio samples and computed metrics for a single tap point.
type TapData struct {
	Stage   Stage
	InstID  channel.ID
	Samples []float64
	PeakDB  float64
	RMSDB   float64
	Active  bool
}

// The span between the first and last samples reaching thresholdFrac of the peak, padded by padFrac: the Chain tab's
// auto-fit (a drum transient is ~2 ms of a ~500 ms capture). Empty or silent returns (0, len), as without auto-fit.
// Single pass, allocation-free.
func ActiveSpan(samples []float64, thresholdFrac, padFrac float64) (start, end int) {
	n := len(samples)
	if n == 0 {
		return 0, 0
	}
	peak := 0.0
	for _, s := range samples {
		a := s
		if a < 0 {
			a = -a
		}
		if a > peak {
			peak = a
		}
	}
	if peak <= 0 {
		return 0, n
	}
	thr := peak * thresholdFrac
	first := -1
	for i := 0; i < n; i++ {
		a := samples[i]
		if a < 0 {
			a = -a
		}
		if a >= thr {
			first = i
			break
		}
	}
	if first < 0 {
		return 0, n
	}
	last := n - 1
	for i := n - 1; i >= 0; i-- {
		a := samples[i]
		if a < 0 {
			a = -a
		}
		if a >= thr {
			last = i
			break
		}
	}
	span := last - first + 1
	pad := int(padFrac * float64(span))
	start = first - pad
	end = last + 1 + pad
	if start < 0 {
		start = 0
	}
	if end > n {
		end = n
	}
	return start, end
}

// Config holds configuration for the scope service.
type Config struct {
	MaxWindowMs int // max capture window in ms (default 500)
	SampleRate  int // audio sample rate
}
