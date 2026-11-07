//go:build js && wasm

package audio

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Only the Sampler edit: the voice itself reaches audio.js as a flattened block.
func init() {
	// Clear forwards the identity edit so JS deletes its entry and re-renders un-edited.
	platformSampleEditChanged = func(id instrument.ID, e SampleEdit) {
		fn := js.Global().Get("updateSampleEdit")
		if !fn.Truthy() {
			return
		}
		fn.Invoke(string(id), string(SampleEditWire(e)))
	}
}
