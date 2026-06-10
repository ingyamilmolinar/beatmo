package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// SampleEditSaveSink is the persistence shim the Sampler-tab Save / Reset
// write the non-destructive sample-edit descriptors through.
// userprefs.SampleEditStore satisfies it; tests use a stub. Mirrors
// RecipeSaveSink (recipe_save_sink.go): write-only surface, reading happens
// at startup via audio.ApplySavedSampleEdits.
type SampleEditSaveSink interface {
	SaveSampleEdit(instID string, fields map[string]float64) error
	DeleteSampleEdit(instID string) error
}

var (
	sampleEditSinkMu sync.RWMutex
	sampleEditSink   SampleEditSaveSink
)

// SetSampleEditSink registers the global sample-edit persistence sink.
// Production bootstraps (cmd/beatmo.go, js_bootstrap_wasm.go) call this
// after constructing the userprefs store. Nil-safe: passing nil keeps the
// in-memory descriptor flow working without persistence.
func SetSampleEditSink(s SampleEditSaveSink) {
	sampleEditSinkMu.Lock()
	sampleEditSink = s
	sampleEditSinkMu.Unlock()
}

func activeSampleEditSink() SampleEditSaveSink {
	sampleEditSinkMu.RLock()
	defer sampleEditSinkMu.RUnlock()
	return sampleEditSink
}

// persistSampleEdit writes the instrument's CURRENT descriptor through the
// sink (samplerState.save() calls it right after audio.SetSampleEdit).
func persistSampleEdit(instID string) {
	sink := activeSampleEditSink()
	if sink == nil {
		return
	}
	if e, ok := audio.SampleEditFor(instID); ok {
		_ = sink.SaveSampleEdit(instID, e.Fields())
	} else {
		// Saving the identity edit cleared the descriptor — mirror that on disk.
		_ = sink.DeleteSampleEdit(instID)
	}
}

// deletePersistedSampleEdit removes the persisted descriptor on Reset.
func deletePersistedSampleEdit(instID string) {
	if sink := activeSampleEditSink(); sink != nil {
		_ = sink.DeleteSampleEdit(instID)
	}
}
