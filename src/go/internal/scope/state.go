// Package scope provides real-time oscilloscope data types and services for A/B pipeline stage comparison.
package scope

// Stage represents a named point in the audio signal chain.
type Stage int

const (
	StageSynth Stage = iota
	StageAntiPop
	StageInsertFX
	StageEQ
	StageSends
	StageMaster
	stageCount
)

// StageLabel returns the short human-readable label for a Stage. These are
// the names shown on stage buttons in the Chain tab; pick short forms so
// they fit in compact buttons. Long-form pedagogical names live in
// StageDescription.
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
	case StageSends:
		return "Bus"
	case StageMaster:
		return "Master"
	default:
		return ""
	}
}

// StageDescription returns the long-form description for a Stage. Used by
// tooltips and the kid-friendly Chain tab to explain what each pipeline
// stage actually does. Empty string for unknown stages.
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
	case StageSends:
		return "Send bus (reverb/delay return)"
	case StageMaster:
		return "Master (post-mix, post-compressor)"
	default:
		return ""
	}
}

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
	InstID  string
	Samples []float64
	PeakDB  float64
	RMSDB   float64
	Active  bool
}

// ActiveSpan returns the [start,end) sample indices that frame the signal
// energy: the first and last samples whose absolute value reaches
// thresholdFrac of the buffer's peak, padded outward by padFrac of that
// span. It is the data behind the Chain tab's "auto-fit" — the scope
// captures ~500ms but a drum transient is ~2ms, so framing the active span
// lets the waveform fill the trace instead of leaving ~98% dead width.
//
// When the buffer is empty or silent (nothing crosses the threshold) it
// returns the full range (0, len) so the caller renders exactly what it
// would have without auto-fit. The walk is single-pass and allocation-free.
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
