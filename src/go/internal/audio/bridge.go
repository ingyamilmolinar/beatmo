//go:build js && wasm
// +build js,wasm

package audio

import "syscall/js"

// exposed from web/audio.js
var (
	play js.Value
)

func init() {
	global := js.Global()
	play = global.Get("play") // (id, when) exported by JS
}

func Play(id string, when float64) {
	// Lazily grab the `play` function if it wasn't available at init time.
	if !play.Truthy() {
		play = js.Global().Get("play")
	}
	if !play.Truthy() {
		js.Global().Get("console").Call("warn", "[audio] play function missing; dropping sample", id)
		return
	}
	play.Invoke(id, when)
}
