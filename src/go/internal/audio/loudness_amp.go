package audio

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// For a runtime instrument without a document (a clone or instance inherits its source's); a shipped one's lives in
// render.amplitude.
func loudnessAmp(id instrument.ID) (float32, bool) {
	if v, ok := runtimeLoudnessAmp.Load(id); ok {
		return v.(float32), true
	}
	return 0, false
}

// So a clone renders byte-identically to its source.
var runtimeLoudnessAmp sync.Map // instrument.ID -> float32 (always that key type)

func setRuntimeLoudnessAmp(id instrument.ID, amp float32) { runtimeLoudnessAmp.Store(id, amp) }
func clearRuntimeLoudnessAmp(id instrument.ID)            { runtimeLoudnessAmp.Delete(id) }
