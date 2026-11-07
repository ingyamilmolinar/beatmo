package ui

import (
	"sync"
)

// Write-only: steps are read once at startup through SetKnobStepSink + LoadKnobSteps.
type KnobStepSaveSink interface {
	// Never nil.
	LoadKnobSteps() map[string]float64
	// SaveKnobStep upserts the chosen step-multiplier for the named param.
	SaveKnobStep(name string, step float64) error
}

var (
	knobStepSinkMu sync.RWMutex
	knobStepSink   KnobStepSaveSink
)

// Nil keeps the in-memory steps working without persistence.
func SetKnobStepSink(s KnobStepSaveSink) {
	knobStepSinkMu.Lock()
	knobStepSink = s
	knobStepSinkMu.Unlock()
}

func activeKnobStepSink() KnobStepSaveSink {
	knobStepSinkMu.RLock()
	defer knobStepSinkMu.RUnlock()
	return knobStepSink
}
