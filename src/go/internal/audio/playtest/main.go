//go:build js && wasm

package main

import (
	"syscall/js"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// main hooks a mousedown event to trigger audio after the context resumes.
func main() {
	js.Global().Set("__wasmReady", false)
	js.Global().Get("document").Call("addEventListener", "mousedown", js.FuncOf(func(js.Value, []js.Value) interface{} {
		audio.Resume()
		js.Global().Call("setTimeout", js.FuncOf(func(js.Value, []js.Value) interface{} {
			js.Global().Set("__playTime", js.Global().Get("__audioCtx").Get("currentTime"))
			audio.Play("snare")
			go func() {
				time.Sleep(250 * time.Millisecond)
				audio.Play("kick")
			}()
			return nil
		}), 0)
		return nil
	}))
	js.Global().Set("__wasmReady", true)
	select {}
}
