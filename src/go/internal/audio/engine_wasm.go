//go:build js && wasm && !test

package audio

import "syscall/js"

type Voice interface{}

type Instrument interface{}

var instruments = []string{"snare", "kick", "hihat", "tom", "clap"}

func Register(id string, inst Instrument) {
	instruments = append(instruments, id)
}

func Play(id string, when ...float64) {
	js.Global().Call("playSound", id)
}

func Now() float64 { return 0 }

func Reset() {}

func Resume() {}

func SetBPM(b int) {}

func Instruments() []string { return instruments }
