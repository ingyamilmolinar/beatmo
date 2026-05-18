//go:build js && wasm

package audio

import (
	"encoding/json"
	"syscall/js"
)

// Phase 5: WASM bridge for per-instrument synth-recipe params. Every
// SetInstrumentParam / SetInstrumentParams / ResetInstrumentParams call
// in the audio manager funnels through the platformInstrumentParamsChanged
// hook; in browser builds we forward the new param map to JS so the audio
// pipeline (voice-buffer cache + the per-instrument param dictionary used
// by the next play) stays in sync.
//
// Mirrors insert_effects_wasm.go's pattern: JSON-marshal the payload and
// invoke a global JS function. The JS side (audio.js:updateInstrumentParams)
// decodes + replays the change against the in-browser voice cache.
func init() {
	platformInstrumentParamsChanged = func(id string, params RecipeParams) {
		fn := js.Global().Get("updateInstrumentParams")
		if !fn.Truthy() {
			return
		}
		data, err := json.Marshal(params)
		if err != nil {
			return
		}
		fn.Invoke(id, string(data))
	}
}
