//go:build js

package audio

import (
	"encoding/json"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Rendered off-thread on the render workers; non-melodic instruments collapse to one render in audio.js. A no-op if the
// JS function is absent.
func warmInstrumentPlatform(id instrument.ID, pitches []int) {
	fn := js.Global().Get("warmInstrument")
	if !fn.Truthy() {
		return
	}
	data, err := json.Marshal(pitches)
	if err != nil {
		return
	}
	fn.Invoke(string(id), string(data))
}
