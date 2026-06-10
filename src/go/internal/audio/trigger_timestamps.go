package audio

import (
	"sync"
	"time"
)

// trigger_timestamps.go — per-instrument "last trigger" timestamp
// tracker. Updated by the Play* entry points (engine_play.go,
// engine_wasm.go, stub.go) whenever a voice is scheduled. The UI
// reads via LastTriggerAt to drive the Synth-tab knob-outline pulse
// glow (Phase 4 audio-panel redesign).

var (
	triggerMu   sync.RWMutex
	triggerByID = map[string]time.Time{}
)

// RecordVoiceTrigger marks the current time as the last-trigger
// timestamp for the supplied instrument ID. Called by the engine's
// Play* family. Safe to call from any goroutine; locking is brief
// (a single map insert under a write lock).
func RecordVoiceTrigger(id string) {
	if id == "" {
		return
	}
	now := time.Now()
	triggerMu.Lock()
	triggerByID[id] = now
	triggerMu.Unlock()
}

// LastTriggerAt returns the most recent voice-trigger timestamp for
// the supplied instrument ID, or the zero time when the instrument
// has never been triggered. Safe to call from any goroutine.
func LastTriggerAt(id string) time.Time {
	triggerMu.RLock()
	defer triggerMu.RUnlock()
	return triggerByID[id]
}

// SinceLastTrigger returns the duration since `id` was last
// triggered, or a very large duration when the instrument has never
// been triggered. Convenience for UI "did this row fire recently"
// checks without explicit zero-time handling.
func SinceLastTrigger(id string) time.Duration {
	t := LastTriggerAt(id)
	if t.IsZero() {
		return 10 * time.Hour
	}
	return time.Since(t)
}

// ResetTriggerTimestamps clears every recorded trigger time. Tests
// call this on setup; production code never reaches it.
func ResetTriggerTimestamps() {
	triggerMu.Lock()
	defer triggerMu.Unlock()
	triggerByID = map[string]time.Time{}
}
