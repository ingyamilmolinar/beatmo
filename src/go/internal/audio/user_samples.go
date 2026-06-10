package audio

import (
	"log"
	"sync"
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
	userSamples   = map[string]SampleRecord{}
	// originalUserSamples keeps the pristine first-loaded PCM per id. Captured
	// once (first-put-wins) so a later edit re-save never moves it; Reset reads
	// it to restore a user WAV/Save-As sample to its original buffer.
	originalUserSamples = map[string]SampleRecord{}
	// sampleOriginRecipe remembers the recipe an instrument was bound to before
	// the Sampler converted it into a sample (the binding is cleared at save
	// time). A factory Reset uses it to re-bind the original synth recipe.
	sampleOriginRecipe = map[string]string{}

	sampleSink     SampleSink
	useSampleStore bool
)

// SampleSink persists/forgets user samples across sessions. Implemented by the
// UI layer over userprefs (desktop files / browser IndexedDB).
type SampleSink interface {
	SaveSample(id string, pcm []float32, sr int)
	DeleteSample(id string)
}

// SampleSource loads previously-persisted user samples at startup.
type SampleSource interface {
	LoadSamples() map[string]SampleRecord
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
func PutUserSample(id string, pcm []float32, sr int) {
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
func OriginalUserSamplePCM(id string) (SampleRecord, bool) {
	userSamplesMu.RLock()
	defer userSamplesMu.RUnlock()
	rec, ok := originalUserSamples[id]
	return rec, ok
}

// RecordSampleOriginRecipe remembers the recipe an instrument was bound to
// before it was converted into a sample. No-op on empty recipeID. Called by the
// Sampler Save flow before it clears the binding so a factory Reset can re-bind.
func RecordSampleOriginRecipe(instID, recipeID string) {
	if instID == "" || recipeID == "" {
		return
	}
	userSamplesMu.Lock()
	sampleOriginRecipe[instID] = recipeID
	userSamplesMu.Unlock()
}

// SampleOriginRecipe returns the recipe an instrument was bound to before sample
// conversion, or "" if none was recorded.
func SampleOriginRecipe(instID string) string {
	userSamplesMu.RLock()
	defer userSamplesMu.RUnlock()
	return sampleOriginRecipe[instID]
}

// forgetUserSample drops all trace of a user sample: the live buffer, the
// pristine original, the origin-recipe record, and the persisted copy.
func forgetUserSample(id string) {
	userSamplesMu.Lock()
	delete(userSamples, id)
	delete(originalUserSamples, id)
	delete(sampleOriginRecipe, id)
	sink := sampleSink
	userSamplesMu.Unlock()
	if sink != nil {
		sink.DeleteSample(id)
	}
}

// ResetSampleToFactory reverts an instrument that was turned into a sample back
// to its factory state. A built-in instrument with a shipped origin recipe is
// re-bound to that recipe (reset to shipped defaults) and its user sample is
// discarded; a user WAV / Save-As sample is restored to its pristine first-
// loaded buffer. Returns (priorRecipeID, factoryReverted): factoryReverted is
// true only for the built-in→synth case. Unknown ids are a no-op.
func ResetSampleToFactory(instID string) (string, bool) {
	if instID == "" {
		return "", false
	}
	// Detect the factory synth from the authoritative shipped binding table
	// first — it is restart-safe and independent of session state, so Reset
	// works even after the app reloaded a persisted sample (which records no
	// session origin and leaves the startup recipe binding in place). Fall back
	// to the session origin record for user-recipe-derived samples.
	origin := factoryRecipeForInstrument(instID)
	if origin == "" {
		origin = SampleOriginRecipe(instID)
	}
	// CASE A: built-in synth that was converted to a sample → full factory revert.
	if origin != "" && len(RecipeShippedDefaults(origin)) > 0 {
		BindInstrumentToRecipe(instID, origin)
		ResetRecipeToShipped(origin)
		ResetInstrumentParams(instID)
		// Restore the playback registration the sample save overwrote (desktop
		// instrument table / WASM render cache) so the synth is actually heard,
		// not just re-bound — then drop the user sample (memory + persistence).
		UnregisterSamplePCM(instID)
		forgetUserSample(instID)
		return origin, true
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

// SaveUserSample stores + registers (PutUserSample) and additionally persists
// the sample via the sink so it survives across sessions. Used by the Sampler
// tab's Save / Save As.
func SaveUserSample(id string, pcm []float32, sr int) {
	PutUserSample(id, pcm, sr)
	userSamplesMu.RLock()
	sink := sampleSink
	userSamplesMu.RUnlock()
	if sink != nil {
		sink.SaveSample(id, append([]float32(nil), pcm...), sr)
	}
}

// UserSamplePCM returns the canonical record for id, if it is a user sample.
func UserSamplePCM(id string) (SampleRecord, bool) {
	userSamplesMu.RLock()
	defer userSamplesMu.RUnlock()
	rec, ok := userSamples[id]
	return rec, ok
}

// IsUserSample reports whether id was created by the Sampler tab (and therefore
// carries embedded PCM the project exporter should serialize).
func IsUserSample(id string) bool {
	userSamplesMu.RLock()
	defer userSamplesMu.RUnlock()
	_, ok := userSamples[id]
	return ok
}

// UserSampleIDs returns the ids of all registered user samples.
func UserSampleIDs() []string {
	userSamplesMu.RLock()
	defer userSamplesMu.RUnlock()
	out := make([]string, 0, len(userSamples))
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
		if recipeID := factoryRecipeForInstrument(id); recipeID != "" {
			log.Printf("[sample] Warning: skipping legacy persisted sample for factory synth %q (recipe %s) — synths remain synths", id, recipeID)
			continue
		}
		PutUserSample(id, rec.PCM, rec.SampleRate)
	}
}
