//go:build js

package audio

import (
	"encoding/json"
	"syscall/js"
)

// warmInstrumentPlatform (browser) forwards the candidate pitch spread to the
// JS render layer, which renders each needed pitch off-thread on the render
// worker(s) and caches it. Non-melodic instruments collapse to a single bare-id
// render inside audio.js (ensureRenderReady ignores pitch for them), so passing
// the full spread is safe. Mirrors synth_recipe_wasm.go's bridge pattern:
// marshal + invoke a global JS function; a no-op if the function is absent.
func warmInstrumentPlatform(id string, pitches []int) {
	fn := js.Global().Get("warmInstrument")
	if !fn.Truthy() {
		return
	}
	data, err := json.Marshal(pitches)
	if err != nil {
		return
	}
	fn.Invoke(id, string(data))
}
