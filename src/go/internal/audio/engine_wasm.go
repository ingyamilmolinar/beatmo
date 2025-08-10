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
