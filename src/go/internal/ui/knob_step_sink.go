package ui

import (
	"sync"
)

// KnobStepSaveSink is the persistence shim the synth/sampler knob step badges
// write through. userprefs.KnobStepStore satisfies it; tests use a stub.
// Mirrors SampleEditSaveSink (sampler_edit_sink.go): write-only surface;
// reading happens at startup via SetKnobStepSink + LoadKnobSteps.
type KnobStepSaveSink interface {
	// LoadKnobSteps returns param-name → step value for every persisted rung.
	// Empty map (never nil) when none exist.
	LoadKnobSteps() map[string]float64
	// SaveKnobStep upserts the chosen step-multiplier for the named param.
	SaveKnobStep(name string, step float64) error
}

var (
	knobStepSinkMu sync.RWMutex
	knobStepSink   KnobStepSaveSink
)

// SetKnobStepSink registers the global knob-step persistence sink.
// Production bootstraps (cmd/beatmo.go, js_bootstrap_wasm.go) call this
// after constructing the userprefs store. Nil-safe: passing nil keeps the
// in-memory step state working without persistence.
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
