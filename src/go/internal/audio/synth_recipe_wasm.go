//go:build js && wasm

package audio

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Non-destructive sample-edit descriptor push (sample_edit_descriptor.go). The
// synth voice itself reaches audio.js as a flattened instrument document block
// (instrument_docs_wasm.go); this file only carries the Sampler edit.
func init() {
	// Set forwards the edit; Clear forwards the identity edit so the JS side
	// (audio.js:updateSampleEdit) deletes its entry and re-renders un-edited.
	platformSampleEditChanged = func(id instrument.ID, e SampleEdit) {
		fn := js.Global().Get("updateSampleEdit")
		if !fn.Truthy() {
			return
		}
		fn.Invoke(string(id), string(SampleEditWire(e)))
	}
}
