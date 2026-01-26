//go:build !test && !js

package audio

import (
	"sync"
	"time"
)
type scaledVoice struct {
	v    Voice
	gain float64
}

func (s *scaledVoice) Sample() (float64, bool) {
	f, done := s.v.Sample()
	return f * s.gain, done
}

// Now returns seconds since program start.
func Now() float64 { return time.Since(start).Seconds() }

// Reset closes the current audio context so queued sounds are dropped.
func Reset() {
	ctx = nil
	mix = nil
	once = sync.Once{}
	resetChannels()
}

// Resume attempts to resume the underlying audio context.
func Resume() {
	once.Do(initContext)
	if ctx != nil {
		_ = ctx.Resume()
	}
}

// SetBPM updates the global tempo used when constructing new voices.
func SetBPM(b int) { bpm = b }

// Instruments returns the list of registered instrument IDs.
func Instruments() []string {
	instMu.RLock()
	ids := append([]string(nil), instOrder...)
	instMu.RUnlock()
	return ids
}

// RenameInstrument updates the ID of an existing instrument.
func RenameInstrument(oldID, newID string) {
	changed := false
	instMu.Lock()
	if inst, ok := instruments[oldID]; ok {
		delete(instruments, oldID)
		instruments[newID] = inst
		for i, id := range instOrder {
			if id == oldID {
				instOrder[i] = newID
				break
			}
		}
		changed = true
	}
	instMu.Unlock()
	if changed {
		bumpInstrumentsVersion()
	}
	renameInstrumentChannel(oldID, newID)
}
