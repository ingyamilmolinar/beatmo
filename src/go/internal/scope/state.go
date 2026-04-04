// Package scope provides real-time oscilloscope data types and services for A/B pipeline stage comparison.
package scope

// Stage represents a named point in the audio signal chain.
type Stage int

const (
	StageSynth    Stage = iota
	StageAntiPop
	StageInsertFX
	StageEQ
	StageSends
	StageMaster
	stageCount
)

// StageLabel returns the human-readable name for a Stage.
func StageLabel(s Stage) string {
	switch s {
	case StageSynth:
		return "Synth"
	case StageAntiPop:
		return "AntiPop"
	case StageInsertFX:
		return "InsertFX"
	case StageEQ:
		return "EQ"
	case StageSends:
		return "Sends"
	case StageMaster:
		return "Master"
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

// Config holds configuration for the scope service.
type Config struct {
	MaxWindowMs int // max capture window in ms (default 500)
	SampleRate  int // audio sample rate
}
