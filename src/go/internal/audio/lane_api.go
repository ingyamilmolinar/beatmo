package audio

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Voice revision: everything that changes the PCM an instrument's NEXT note
// renders. The audio lane folds it into each committed note's identity, so a
// synth / sampler edit replaces that instrument's not-yet-started notes even
// though they were committed up to the horizon ahead.
var (
	voiceRevMu sync.Mutex
	voiceRevX  = map[instrument.ID]uint64{}
)

// bumpVoiceRev records a PCM-affecting change that InstrumentParamsRev does
// not cover (sample edits, PCM registration).
func bumpVoiceRev(inst instrument.ID) {
	voiceRevMu.Lock()
	voiceRevX[inst]++
	voiceRevMu.Unlock()
}

// VoiceRevision combines the instrument document revision and the sample-edit /
// PCM revision of inst.
func VoiceRevision(inst instrument.ID) uint64 {
	voiceRevMu.Lock()
	x := voiceRevX[inst]
	voiceRevMu.Unlock()
	return InstrumentParamsRev(inst)*1_000_003 + x
}
