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

func (s *scaledVoice) SampleBlock(dst []float64) (int, bool) {
	if bv, ok := s.v.(BlockVoice); ok {
		n, done := bv.SampleBlock(dst)
		g := s.gain
		for i := 0; i < n; i++ {
			dst[i] *= g
		}
		return n, done
	}
	for i := range dst {
		val, done := s.v.Sample()
		if done {
			return i, true
		}
		dst[i] = val * s.gain
	}
	// Probe: voice may be exactly exhausted at block boundary.
	if _, done := s.v.Sample(); done {
		return len(dst), true
	}
	return len(dst), false
}

// nowOverride allows tests to inject a deterministic audio clock.
var nowOverride func() float64

// Now returns seconds since program start.
func Now() float64 {
	if nowOverride != nil {
		return nowOverride()
	}
	return time.Since(start).Seconds()
}

// SetNowForTest overrides audio.Now() to return values from fn.
// Returns a restore function.
func SetNowForTest(fn func() float64) func() {
	old := nowOverride
	nowOverride = fn
	return func() { nowOverride = old }
}

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
