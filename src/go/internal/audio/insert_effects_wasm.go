//go:build js && wasm

package audio

import (
	"encoding/json"
	"syscall/js"
)

func init() {
	platformInsertEffectsChanged = func(id string, slots []EffectSlot) {
		fn := js.Global().Get("updateInsertEffects")
		if !fn.Truthy() {
			return
		}
		// Marshal slots to JSON and pass as string for simplicity.
		// JS side parses it back.
		data, err := json.Marshal(slots)
		if err != nil {
			return
		}
		fn.Invoke(id, string(data))
	}
}
