package audio

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Everything that changes the PCM of an instrument's NEXT note: the lane folds it into each committed note's identity,
// so an edit replaces notes committed up to the horizon ahead.
var (
	voiceRevMu sync.Mutex
	voiceRevX  = map[instrument.ID]uint64{}
)

// For PCM changes InstrumentParamsRev does not cover (sample edits, PCM registration).
func bumpVoiceRev(inst instrument.ID) {
	voiceRevMu.Lock()
	voiceRevX[inst]++
	voiceRevMu.Unlock()
}

func VoiceRevision(inst instrument.ID) uint64 {
	voiceRevMu.Lock()
	x := voiceRevX[inst]
	voiceRevMu.Unlock()
	return InstrumentParamsRev(inst)*1_000_003 + x
}
