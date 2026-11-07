package audio

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// The canonical Go-side PCM of Sampler-made samples: the exporter and the persistence sink both read it here, so neither
// needs to get bytes back out of an audio engine. RegisterSamplePCM handles playback registration.

// SampleRecord is a baked, self-contained mono PCM buffer plus its rate.
type SampleRecord struct {
	PCM        []float32
	SampleRate int
}

var (
	userSamplesMu sync.RWMutex
	userSamples   = map[instrument.ID]SampleRecord{}
	// First-put-wins, so a later re-save never moves the baseline a reset restores.
	originalUserSamples = map[instrument.ID]SampleRecord{}

	sampleSink     SampleSink
	useSampleStore bool
)

// Implemented by the UI over userprefs (desktop files / browser IndexedDB).
type SampleSink interface {
	SaveSample(id instrument.ID, pcm []float32, sr int)
	DeleteSample(id instrument.ID)
}

// SampleSource loads previously-persisted user samples at startup.
type SampleSource interface {
	LoadSamples() map[instrument.ID]SampleRecord
}

// SetSampleSink installs the persistence sink and returns a restore func.
func SetSampleSink(s SampleSink) func() {
	userSamplesMu.Lock()
	prev := sampleSink
	sampleSink = s
	userSamplesMu.Unlock()
	return func() {
		userSamplesMu.Lock()
		sampleSink = prev
		userSamplesMu.Unlock()
	}
}

// When off, ApplySavedSamples is a no-op.
func SetUseSampleStore(on bool) func() {
	userSamplesMu.Lock()
	prev := useSampleStore
	useSampleStore = on
	userSamplesMu.Unlock()
	return func() {
		userSamplesMu.Lock()
		useSampleStore = prev
		userSamplesMu.Unlock()
	}
}

// UseSampleStore reports the current gate state.
func UseSampleStore() bool {
	userSamplesMu.RLock()
	defer userSamplesMu.RUnlock()
	return useSampleStore
}

// Does NOT persist (SaveUserSample does): for import and startup re-registration of already-persisted data.
func PutUserSample(id instrument.ID, pcm []float32, sr int) {
	cp := append([]float32(nil), pcm...)
	userSamplesMu.Lock()
	userSamples[id] = SampleRecord{PCM: cp, SampleRate: sr}
	// Captured once, so later re-saves never overwrite the baseline.
	if _, exists := originalUserSamples[id]; !exists {
		originalUserSamples[id] = SampleRecord{PCM: append([]float32(nil), cp...), SampleRate: sr}
	}
	userSamplesMu.Unlock()
	RegisterSamplePCM(id, cp, sr)
}

// OriginalUserSamplePCM returns the pristine first-loaded record for id, if any.
func OriginalUserSamplePCM(id instrument.ID) (SampleRecord, bool) {
	userSamplesMu.RLock()
	defer userSamplesMu.RUnlock()
	rec, ok := originalUserSamples[id]
	return rec, ok
}

// The live buffer, the pristine original and the persisted copy.
func forgetUserSample(id instrument.ID) {
	userSamplesMu.Lock()
	delete(userSamples, id)
	delete(originalUserSamples, id)
	sink := sampleSink
	userSamplesMu.Unlock()
	if sink != nil {
		sink.DeleteSample(id)
	}
}

// For this SESSION only: import is replace-not-merge, so a sample the PREVIOUS project put on a built-in must not
// survive into a project whose row is the synth. Touches neither persistence nor the document; user samples are left
// alone (a file refers to them by id).
func DropBuiltinSampleOverride(instID instrument.ID) bool {
	if instID == "" || !isShippedInstrument(instID) {
		return false
	}
	userSamplesMu.Lock()
	_, ok := userSamples[instID]
	if ok {
		delete(userSamples, instID)
		delete(originalUserSamples, instID)
	}
	userSamplesMu.Unlock()
	if !ok {
		return false
	}
	UnregisterSamplePCM(instID)
	return true
}

// A library-document instrument returns to its synth (copy to its library entry, user sample discarded); a user WAV to
// its pristine buffer. factoryReverted is true only for the synth case.
func ResetSampleToFactory(instID instrument.ID) (instrument.ID, bool) {
	if instID == "" {
		return "", false
	}
	// CASE A: a synth instrument that was converted to a sample → full revert.
	if lib, ok := libraryCopyOf(instID); ok {
		ResetInstrumentParams(instID)
		// Restore the playback registration the sample overwrote so the synth is actually heard.
		UnregisterSamplePCM(instID)
		forgetUserSample(instID)
		return lib.Origin, true
	}
	// CASE B: user WAV / Save-As sample → restore the pristine first-loaded buffer.
	if orig, ok := OriginalUserSamplePCM(instID); ok && len(orig.PCM) > 0 {
		PutUserSample(instID, orig.PCM, orig.SampleRate)
		userSamplesMu.RLock()
		sink := sampleSink
		userSamplesMu.RUnlock()
		if sink != nil {
			sink.SaveSample(instID, append([]float32(nil), orig.PCM...), orig.SampleRate)
		}
		return "", false
	}
	return "", false
}

func isShippedInstrument(id instrument.ID) bool { return roster.Has(id.Base()) }

// Persisted via the sink so it survives sessions.
func SaveUserSample(id instrument.ID, pcm []float32, sr int) {
	PutUserSample(id, pcm, sr)
	userSamplesMu.RLock()
	sink := sampleSink
	userSamplesMu.RUnlock()
	if sink != nil {
		sink.SaveSample(id, append([]float32(nil), pcm...), sr)
	}
}

// Without re-registering for playback: the source's edit already drives playback through the descriptor, and
// re-registering the pristine buffer would clobber it.
func PersistUserSample(id instrument.ID) {
	userSamplesMu.RLock()
	rec, ok := userSamples[id]
	sink := sampleSink
	userSamplesMu.RUnlock()
	if !ok || len(rec.PCM) == 0 || sink == nil {
		return
	}
	sink.SaveSample(id, append([]float32(nil), rec.PCM...), rec.SampleRate)
}

// UserSamplePCM returns the canonical record for id, if it is a user sample.
func UserSamplePCM(id instrument.ID) (SampleRecord, bool) {
	userSamplesMu.RLock()
	defer userSamplesMu.RUnlock()
	rec, ok := userSamples[id]
	return rec, ok
}

// Its PCM is embedded by the project exporter.
func IsUserSample(id instrument.ID) bool {
	userSamplesMu.RLock()
	defer userSamplesMu.RUnlock()
	_, ok := userSamples[id]
	return ok
}

// UserSampleIDs returns the ids of all registered user samples.
func UserSampleIDs() []instrument.ID {
	userSamplesMu.RLock()
	defer userSamplesMu.RUnlock()
	out := make([]instrument.ID, 0, len(userSamples))
	for id := range userSamples {
		out = append(out, id)
	}
	return out
}

// Ids on a shipped instrument are SKIPPED: synths remain synths. Re-registering such a stale sample would override the
// synth's playback and, in the browser, freeze it so every Synth-tab edit is a no-op.
func ApplySavedSamples(src SampleSource) {
	if src == nil || !UseSampleStore() {
		return
	}
	for id, rec := range src.LoadSamples() {
		if id == "" || len(rec.PCM) == 0 {
			continue
		}
		if isShippedInstrument(id) {
			log.Warnf("[sample] skipping legacy persisted sample for shipped synth %q — synths remain synths", id)
			continue
		}
		PutUserSample(id, rec.PCM, rec.SampleRate)
	}
}
