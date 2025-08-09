//go:build js && wasm

package main

import (
	"syscall/js"
	"time"

	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

// main hooks a mousedown event to trigger audio after the context resumes.
func main() {
	js.Global().Set("__wasmReady", false)
	perf := js.Global().Get("performance")
	js.Global().Get("document").Call("addEventListener", "mousedown", js.FuncOf(func(js.Value, []js.Value) any {
		js.Global().Set("__playTime", perf.Call("now"))
		audio.Play("snare")
		go func() {
			time.Sleep(250 * time.Millisecond)
			audio.Play("snare")
		}()
		return nil
	}))
	js.Global().Set("__wasmReady", true)
	select {}
}
