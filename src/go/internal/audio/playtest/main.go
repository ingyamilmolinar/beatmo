//go:build js && wasm

package main

import (
	"syscall/js"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// main hooks a mousedown event to trigger audio after the context resumes.
func main() {
	js.Global().Set("__wasmReady", false)
	js.Global().Get("document").Call("addEventListener", "mousedown", js.FuncOf(func(js.Value, []js.Value) interface{} {
		audio.Resume()
		// The browser has no instrument until Go pushes its document (the full
		// app does it at boot): push the two this harness plays.
		for _, id := range []instrument.ID{roster.Snare, roster.Kick} {
			audio.EnsureInstrumentDoc(id)
		}
		audio.PushAllInstrumentDocsToPlatform()
		js.Global().Call("setTimeout", js.FuncOf(func(js.Value, []js.Value) interface{} {
			js.Global().Set("__playTime", js.Global().Get("__audioCtx").Get("currentTime"))
			audio.Play(roster.Snare)
			go func() {
				time.Sleep(250 * time.Millisecond)
				audio.Play(roster.Kick)
			}()
			return nil
		}), 0)
		return nil
	}))
	js.Global().Set("__wasmReady", true)
	select {}
}
