package audio

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// loudnessAmp returns the playback amplitude recorded for a runtime instrument
// without a document (a clone / instance alias inherits its source's), and
// whether one exists. A shipped instrument's amplitude lives in its document's
// render.amplitude (ConfigForInstrument reads it first); there is no table.
func loudnessAmp(id instrument.ID) (float32, bool) {
	if v, ok := runtimeLoudnessAmp.Load(id); ok {
		return v.(float32), true
	}
	return 0, false
}

// runtimeLoudnessAmp holds the amplitude of ids created at runtime (clones), so
// a clone reports the same amplitude as its source and renders byte-identically.
var runtimeLoudnessAmp sync.Map // instrument.ID -> float32 (always that key type)

// setRuntimeLoudnessAmp records the loudness amp a runtime instrument should
// use (its source's measured amp). clearRuntimeLoudnessAmp drops it on delete.
func setRuntimeLoudnessAmp(id instrument.ID, amp float32) { runtimeLoudnessAmp.Store(id, amp) }
