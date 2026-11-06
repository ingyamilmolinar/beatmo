//go:build test

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// Audio-lane engine operations are no-ops in the stub build: UI tests drive
// the lane through a fake audioSink.

// LaneEngineHooks let -tags test code observe (and answer) the lane's engine
// calls: Cancel returns the vids the "engine" refuses as already started.
type LaneEngineHooks struct {
	Cancel    func(ids []uint64) (started []uint64)
	StopAt    func(inst instrument.ID, t float64)
	CancelAll func()
}

var laneEngineHooks LaneEngineHooks

// SetLaneEngineHooksForTest installs h and returns a restore func.
func SetLaneEngineHooksForTest(h LaneEngineHooks) (restore func()) {
	prev := laneEngineHooks
	laneEngineHooks = h
	return func() { laneEngineHooks = prev }
}

func CancelVoices(ids []uint64) (started []uint64) {
	if h := laneEngineHooks.Cancel; h != nil {
		return h(ids)
	}
	return nil
}
func StopLaneAt(inst instrument.ID, t float64) {
	if h := laneEngineHooks.StopAt; h != nil {
		h(inst, t)
	}
}
func CancelAllLaneFuture() {
	if h := laneEngineHooks.CancelAll; h != nil {
		h()
	}
}
