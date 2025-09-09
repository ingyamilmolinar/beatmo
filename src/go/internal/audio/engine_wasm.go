//go:build js && wasm && !test

package audio

import (
	"sync"
	"syscall/js"
)

type Voice interface{}

type Instrument interface{}

var (
	instruments   = []string{"snare", "kick", "hihat", "tom", "clap"}
	instrumentsMu sync.RWMutex
)

func Register(id string, inst Instrument) {
	instrumentsMu.Lock()
	instruments = append(instruments, id)
	instrumentsMu.Unlock()
}

func Play(id string, when ...float64) {
	fn := js.Global().Get("playSound")
	if !fn.Truthy() {
		return
	}
	if len(when) > 0 {
		fn.Invoke(id, 1.0, when[0])
		return
	}
	fn.Invoke(id, 1.0)
}

// PlayVol plays an instrument at the given volume. Volume is forwarded
// to the WebAudio bridge which applies a GainNode before the destination.
func PlayVol(id string, vol float64, when ...float64) {
	// Pass id and volume to JS. Optionally pass the first 'when' timestamp
	// if provided (seconds in AudioContext time). The JS side may ignore it.
	fn := js.Global().Get("playSound")
	if !fn.Truthy() {
		return
	}
	if len(when) > 0 {
		fn.Invoke(id, vol, when[0])
		return
	}
	fn.Invoke(id, vol)
}

// ResetInstruments restores the default instrument ID list.
func ResetInstruments() {
	instrumentsMu.Lock()
	instruments = []string{"snare", "kick", "hihat", "tom", "clap"}
	instrumentsMu.Unlock()
}

// Now returns the current WebAudio time in seconds as reported by
// AudioContext.currentTime via the JS bridge. Falls back to 0 when unavailable.
func Now() float64 {
	// Guard against missing bridge function to avoid panics when the
	// page hasn't loaded audio.js yet. Returning 0 signals callers to
	// schedule immediately using the current AudioContext time.
	fn := js.Global().Get("audioNow")
	if !fn.Truthy() {
		return 0
	}
	v := fn.Invoke()
	if v.Truthy() {
		return v.Float()
	}
	return 0
}

func Reset() {}

func Resume() {
	// Ask JS to resume the AudioContext, typically after a user gesture.
	if fn := js.Global().Get("resumeAudio"); fn.Truthy() {
		fn.Invoke()
	}
}

func SetBPM(b int) {}

func Instruments() []string {
	instrumentsMu.RLock()
	ids := append([]string(nil), instruments...)
	instrumentsMu.RUnlock()
	return ids
}

// RenameInstrument updates an instrument ID in the list.
func RenameInstrument(oldID, newID string) {
	instrumentsMu.Lock()
	for i, id := range instruments {
		if id == oldID {
			instruments[i] = newID
			break
		}
	}
	instrumentsMu.Unlock()
}
