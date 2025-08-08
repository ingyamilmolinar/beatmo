//go:build js && wasm
// +build js,wasm

package audio

import "syscall/js"

// exposed from web/audio.js
var (
	play js.Value
	ctx  js.Value
)

func init() {
	global := js.Global()
	play = global.Get("play") // (id, when) exported by JS
	ctx = global.Get("audioCtx")
}

// Now returns the AudioContext's currentTime.
func Now() float64 {
	if ctx.Truthy() {
		return ctx.Get("currentTime").Float()
	}
	return 0
}

func Play(id string, when ...float64) {
	// Lazily grab the `play` function if it wasn't available at init time.
	if !play.Truthy() {
		play = js.Global().Get("play")
	}
	if !play.Truthy() {
		js.Global().Get("console").Call("warn", "[audio] play function missing; dropping sample", id)
		return
	}
	go func() {
		if len(when) > 0 {
			play.Invoke(id, when[0])
		} else {
			play.Invoke(id)
		}
	}()
}
