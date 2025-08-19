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
	js.Global().Call("playSound", id)
}

// PlayVol plays an instrument at the given volume. The current
// WebAudio bridge does not support volume, so the parameter is
// ignored for now.
func PlayVol(id string, vol float64, when ...float64) {
	js.Global().Call("playSound", id)
}

// ResetInstruments restores the default instrument ID list.
func ResetInstruments() {
	instrumentsMu.Lock()
	instruments = []string{"snare", "kick", "hihat", "tom", "clap"}
	instrumentsMu.Unlock()
}

func Now() float64 { return 0 }

func Reset() {}

func Resume() {}

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
