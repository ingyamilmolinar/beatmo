package audio

import (
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Updated by every Play* entry point; the UI reads it for the Synth-tab trigger glow.

var (
	triggerMu   sync.RWMutex
	triggerByID = map[instrument.ID]time.Time{}
)

// Safe from any goroutine.
func RecordVoiceTrigger(id instrument.ID) {
	if id == "" {
		return
	}
	now := time.Now()
	triggerMu.Lock()
	triggerByID[id] = now
	triggerMu.Unlock()
}

// Zero time when never triggered.
func LastTriggerAt(id instrument.ID) time.Time {
	triggerMu.RLock()
	defer triggerMu.RUnlock()
	return triggerByID[id]
}

// A very large duration when never triggered, so callers need no zero-time check.
func SinceLastTrigger(id instrument.ID) time.Duration {
	t := LastTriggerAt(id)
	if t.IsZero() {
		return 10 * time.Hour
	}
	return time.Since(t)
}

// Test setup only.
func ResetTriggerTimestamps() {
	triggerMu.Lock()
	defer triggerMu.Unlock()
	triggerByID = map[instrument.ID]time.Time{}
}
