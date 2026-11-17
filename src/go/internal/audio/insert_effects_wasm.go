//go:build js && wasm

package audio

import (
	"encoding/json"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// The browser runs every insert effect in the C engine inside the beatmo-dsp
// worklet (dsp_host.js). A structural edit pushes the whole chain — instance
// uids, types, enabled flags and complete parameter sets — and the worklet
// keeps the C instance of every uid it already holds (desktop keeps the same
// Go processors); a param edit retunes one live instance.
func init() {
	platformResetDSPState = func() {
		if fn := js.Global().Get("resetDSPState"); fn.Truthy() {
			fn.Invoke()
		}
	}
	platformInsertEffectsChanged = func(id channel.ID, chain []InsertEffectInstance) {
		if id.IsZero() {
			return // names no channel: never "" to JS, which reads it as the master bus
		}
		fn := js.Global().Get("updateInsertEffects")
		if !fn.Truthy() {
			return
		}
		if chain == nil {
			chain = []InsertEffectInstance{}
		}
		data, err := json.Marshal(chain)
		if err != nil {
			return
		}
		fn.Invoke(id.String(), string(data))
	}
	platformInsertEffectParamChanged = func(id channel.ID, uid uint64, param string, value float64) {
		if id.IsZero() {
			return
		}
		fn := js.Global().Get("retuneInsertEffect")
		if !fn.Truthy() {
			return
		}
		fn.Invoke(id.String(), float64(uid), param, value)
	}
}
