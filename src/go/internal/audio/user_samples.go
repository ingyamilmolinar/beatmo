package audio

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// User-sample registry: the canonical Go-side store of PCM for samples created
// by the Sampler tab (synth-capture or WAV-load, baked). It is the single
// source of truth that the project exporter embeds and the persistence sink
// writes — both read PCM from here, so neither needs a platform-specific
// "get the bytes back out of the audio engine" path (the sampler always hands
// us the baked []float32 at save time). RegisterSamplePCM still handles
// platform playback registration (native Sample, browser AudioBuffer).

// SampleRecord is a baked, self-contained mono PCM buffer plus its rate.
type SampleRecord struct {
	PCM        []float32
	SampleRate int
}

var (
	userSamplesMu sync.RWMutex
	userSamples   = map[instrument.ID]SampleRecord{}
	// originalUserSamples keeps the pristine first-loaded PCM per id. Captured
	// once (first-put-wins) so a later edit re-save never moves it; Reset reads
	// it to restore a user WAV/Save-As sample to its original buffer.
	originalUserSamples = map[instrument.ID]SampleRecord{}

	sampleSink     SampleSink
	useSampleStore bool
)

// SampleSink persists/forgets user samples across sessions. Implemented by the
// UI layer over userprefs (desktop files / browser IndexedDB).
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

// SetUseSampleStore toggles the cross-session persistence parity gate and
// returns a restore func. When off, ApplySavedSamples is a no-op.
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

// PutUserSample stores the baked PCM as the canonical buffer for id and
// registers it for playback. It does NOT persist — use SaveUserSample for that.
// Used by project import and startup re-registration (already-persisted data).
func PutUserSample(id instrument.ID, pcm []float32, sr int) {
	cp := append([]float32(nil), pcm...)
	userSamplesMu.Lock()
	userSamples[id] = SampleRecord{PCM: cp, SampleRate: sr}
	// First-put-wins: capture the pristine original exactly once so later edit
	// re-saves don't overwrite the baseline a factory Reset restores to.
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

// forgetUserSample drops all trace of a user sample: the live buffer, the
// pristine original, and the persisted copy.
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

// DropBuiltinSampleOverride removes, for this SESSION, a sample registered over
// a built-in synth instrument, so the instrument plays its synth again. Import
// calls it for every built-in row whose file entry carries no PCM: import is
// replace-not-merge, and a sample the PREVIOUS project put on "snare" must not
// survive into a project whose snare is the synth. Unlike ResetSampleToFactory
// it touches neither persistence nor the recipe (a Synth-tab Save on the
// recipe is not the file's to undo). Non-built-in ids (user WAV / Save-As
// samples) are left alone — a file refers to those by id. Reports whether an
// override was dropped.
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

// ResetSampleToFactory reverts an instrument that was turned into a sample back
// to its factory state. An instrument with a library document (a shipped one,
// or a user library instrument) returns to its synth: the project copy goes
// back to its library entry and the user sample is discarded. A user WAV
// sample is restored to its pristine first-loaded buffer. Returns (the library
// id the synth came back from, factoryReverted); factoryReverted is true only
// for the synth case. Unknown ids are a no-op.
func ResetSampleToFactory(instID instrument.ID) (instrument.ID, bool) {
	if instID == "" {
		return "", false
	}
	// CASE A: a synth instrument that was converted to a sample → full revert.
	if lib, ok := libraryCopyOf(instID); ok {
		ResetInstrumentParams(instID)
		// Restore the playback registration the sample save overwrote (desktop
		// instrument table / WASM render cache) so the synth is actually heard
		// — then drop the user sample (memory + persistence).
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

// isShippedInstrument reports whether id (or its instance base) is a shipped
// synth instrument.
func isShippedInstrument(id instrument.ID) bool { return roster.Has(id.Base()) }

// SaveUserSample stores + registers (PutUserSample) and additionally persists
// the sample via the sink so it survives across sessions. Used by the Sampler
// tab's Save / Save As.
func SaveUserSample(id instrument.ID, pcm []float32, sr int) {
	PutUserSample(id, pcm, sr)
	userSamplesMu.RLock()
	sink := sampleSink
	userSamplesMu.RUnlock()
	if sink != nil {
		sink.SaveSample(id, append([]float32(nil), pcm...), sr)
	}
}

// PersistUserSample writes the instrument's canonical (pristine) PCM to the
// persistence sink WITHOUT re-registering it for playback. The Sampler Save on a
// WAV / user-sample source uses it: that source's edit already drives playback
// in real time through the sample-edit descriptor (reapplyUserSampleEdit
// re-registers the BAKED buffer), so re-registering the pristine buffer here —
// as SaveUserSample/PutUserSample would — must NOT happen or it would clobber
// the edited playback. No-op when id is not a stored user sample or no sink is
// installed.
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

// IsUserSample reports whether id was created by the Sampler tab (and therefore
// carries embedded PCM the project exporter should serialize).
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

// ApplySavedSamples re-registers every persisted user sample for playback at
// startup. Gated on UseSampleStore(); mirrors ApplyUserRecipeOverrides.
//
// Factory-recipe-bound ids (e.g. "snare") are SKIPPED: synths remain synths.
// A persisted sample under a builtin id is a legacy artifact of the old
// destructive Sampler-Save (pre sample-edit descriptors). Re-registering it
// would override the synth's playback registration — on WASM that deletes
// RENDER[id] and freezes the instrument, making every Synth-tab edit audibly
// a no-op while desktop (whose recipe dispatch wins once params exist) keeps
// responding: the user-visible "knob/stage/Save changes do nothing in the
// browser" divergence. Regression: TestApplySavedSamplesSkipsFactorySynths +
// synth_edits_override_stale_sample.browser.test.js.
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
