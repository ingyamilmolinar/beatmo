//go:build js && wasm && !test

package audio

import "syscall/js"

type Voice interface{}

type Instrument interface{}

func Register(id string, inst Instrument) {}

func Play(id string, when ...float64) {
	js.Global().Call("playSound", id)
}

func Now() float64 { return 0 }

func Reset() {}

func Resume() {}

func SetBPM(b int) {}

func Instruments() []string { return []string{"snare", "kick"} }
