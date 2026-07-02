package audio

import (
	"math"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// Per-instrument non-destructive sample-edit descriptors.
//
// A descriptor keeps a synth instrument a SYNTH: instead of baking the
// Sampler tab's edit into PCM and clearing the recipe binding (which made
// later synth changes silently inaudible), Save stores the SampleEdit here
// and the voice dispatcher applies it to the freshly-rendered recipe buffer
// at trigger time (tryRecipeVoice → ApplySampleEditToBuffer). Synth knob
// edits, recipe Saves, and descriptor changes all funnel through the same
// next-trigger model: mutate → voiceCacheInvalidate → next hit re-renders.
//
// This file is build-tag-free so the registry exists identically on native,
// WASM, and -tags test builds (the same contract as sample_edit.go).

var (
	sampleEditsMu sync.RWMutex
	sampleEdits   = map[string]SampleEdit{}
)

// platformSampleEditChanged is invoked after every Set/Clear so platform
// bridges (WASM → window.updateSampleEdit) can sync state. Clear pushes the
// identity edit so the JS side deletes its entry. Default is a no-op; the
// js+wasm build overrides it from synth_recipe_wasm.go. Mirrors
// platformInstrumentParamsChanged (synth_recipe.go:172).
var platformSampleEditChanged = func(id string, e SampleEdit) {}

// SwapPlatformSampleEditChangedForTest replaces the platform callback hook
// and returns the previous value, mirroring
// SwapPlatformInstrumentParamsChangedForTest.
func SwapPlatformSampleEditChangedForTest(fn func(id string, e SampleEdit)) func(id string, e SampleEdit) {
	prev := platformSampleEditChanged
	if fn != nil {
		platformSampleEditChanged = fn
	}
	return prev
}

// SetSampleEdit stores the non-destructive edit descriptor for an instrument,
// invalidates its cached voices (next trigger re-renders through the edit),
// and pushes the descriptor to the platform bridge. Storing the identity
// edit is equivalent to ClearSampleEdit.
func SetSampleEdit(instID string, e SampleEdit) {
	if e.isIdentity() {
		ClearSampleEdit(instID)
		return
	}
	sampleEditsMu.Lock()
	// No-op skip: re-applying the identical edit (as a full re-import does on
	// every undo/redo) would re-render the sample for nothing.
	if cur, ok := sampleEdits[instID]; ok && cur == e {
		sampleEditsMu.Unlock()
		return
	}
	sampleEdits[instID] = e
	sampleEditsMu.Unlock()

	voiceCacheInvalidate(instID)
	reapplyUserSampleEdit(instID, e)
	hooks.PublishKind(hooks.EventSampleEditChanged, hooks.SamplePayload{SampleID: instID})
	platformSampleEditChanged(instID, e)
}

// reapplyUserSampleEdit makes a Sampler edit on a WAV / user-sample instrument
// audible IMMEDIATELY — the real-time, no-Save path. A user sample has no synth
// recipe to re-render, so the edit cannot be applied through the recipe voice
// dispatch (tryRecipeVoice); instead we re-derive the playable PCM from the
// instrument's PRISTINE source through descriptor e and re-register it via
// RegisterSamplePCM. That one bridge updates the native Sample table AND the
// browser renderCache, so the edited chop plays on the next trigger on every
// platform without baking destructively (the pristine stays the source of truth
// in the user-sample store, so further edits and Reset are non-destructive).
//
// No-op for instruments without a stored pristine source — i.e. synths, which
// re-render through the recipe path and apply the descriptor there instead.
//
// The source is UserSamplePCM (the CURRENT canonical pristine), not
// OriginalUserSamplePCM (first-put-wins): project import calls PutUserSample with
// the embedded pristine PCM and THEN SetSampleEdit, so baking from the current
// store re-derives the playable buffer from the freshly imported source —
// OriginalUserSamplePCM could still hold a prior session's first-loaded buffer.
func reapplyUserSampleEdit(id string, e SampleEdit) {
	rec, ok := UserSamplePCM(id)
	if !ok || len(rec.PCM) == 0 {
		return
	}
	sr := rec.SampleRate
	if sr <= 0 {
		sr = SampleRate()
	}
	RegisterSamplePCM(id, BakeSample(rec.PCM, sr, e), sr)
}

// ClearSampleEdit removes the descriptor for an instrument. A no-op (no
// invalidation, no bridge push) when no descriptor exists.
func ClearSampleEdit(instID string) {
	sampleEditsMu.Lock()
	_, had := sampleEdits[instID]
	delete(sampleEdits, instID)
	sampleEditsMu.Unlock()
	if !had {
		return
	}
	voiceCacheInvalidate(instID)
	// Restore a user sample's pristine playback (identity bake) so clearing the
	// edit reverts the chop in real time, mirroring SetSampleEdit. No-op for synths.
	reapplyUserSampleEdit(instID, SampleEdit{StartFrac: 0, EndFrac: 1})
	hooks.PublishKind(hooks.EventSampleEditChanged, hooks.SamplePayload{SampleID: instID})
	platformSampleEditChanged(instID, SampleEdit{EndFrac: 1}) // identity ⇒ JS deletes its entry
}

// SampleEditFor returns the descriptor for an instrument and whether one
// exists.
func SampleEditFor(instID string) (SampleEdit, bool) {
	sampleEditsMu.RLock()
	defer sampleEditsMu.RUnlock()
	e, ok := sampleEdits[instID]
	return e, ok
}

// HasSampleEdit reports whether a descriptor exists for an instrument —
// the dispatch-gate helper for tryRecipeVoice.
func HasSampleEdit(instID string) bool {
	sampleEditsMu.RLock()
	defer sampleEditsMu.RUnlock()
	_, ok := sampleEdits[instID]
	return ok
}

// hashSampleEdit returns a deterministic FNV-1a fingerprint of the edit,
// folded into the voice-cache paramsHash so descriptor changes miss the
// cache exactly like param changes do.
func hashSampleEdit(e SampleEdit) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	mix := func(v uint64) {
		for i := 0; i < 8; i++ {
			h ^= v & 0xff
			h *= prime64
			v >>= 8
		}
	}
	mix(math.Float64bits(e.StartFrac))
	mix(math.Float64bits(e.EndFrac))
	mix(math.Float64bits(e.TransposeSemis))
	mix(math.Float64bits(e.DetuneCents))
	mix(uint64(math.Float32bits(e.GainDB)))
	mix(math.Float64bits(e.FadeInMs))
	mix(math.Float64bits(e.FadeOutMs))
	var flags uint64
	if e.Reverse {
		flags |= 1
	}
	if e.Normalize {
		flags |= 2
	}
	mix(flags)
	return h
}

// SampleEditSignature returns a stable fingerprint of an instrument's
// descriptor (0 when none). The Sampler tab folds it into its source
// signature so an externally-changed descriptor (import, rehydration)
// re-syncs the editor.
func SampleEditSignature(instID string) uint64 {
	e, ok := SampleEditFor(instID)
	if !ok {
		return 0
	}
	return hashSampleEdit(e)
}

// Fields encodes the edit as a flat name→value map (bools as 0/1) — the
// persistence shape userprefs.SampleEditStore stores inside prefs.json.
// The audio package owns this conversion (mirroring how RecipeDoc bytes
// stay opaque to userprefs).
func (e SampleEdit) Fields() map[string]float64 {
	b2f := func(b bool) float64 {
		if b {
			return 1
		}
		return 0
	}
	return map[string]float64{
		"start_frac":      e.StartFrac,
		"end_frac":        e.EndFrac,
		"transpose_semis": e.TransposeSemis,
		"detune_cents":    e.DetuneCents,
		"gain_db":         float64(e.GainDB),
		"fade_in_ms":      e.FadeInMs,
		"fade_out_ms":     e.FadeOutMs,
		"reverse":         b2f(e.Reverse),
		"normalize":       b2f(e.Normalize),
	}
}

// SampleEditFromFields is the inverse of Fields. Unknown keys are ignored;
// missing keys take their zero value (EndFrac included — persisted edits
// always carry it, and a zero EndFrac is caught by isIdentity-adjacent
// sanity at apply time: BakeSample yields an empty buffer, audibly wrong
// but never unsafe).
func SampleEditFromFields(fields map[string]float64) SampleEdit {
	return SampleEdit{
		StartFrac:      fields["start_frac"],
		EndFrac:        fields["end_frac"],
		TransposeSemis: fields["transpose_semis"],
		DetuneCents:    fields["detune_cents"],
		GainDB:         float32(fields["gain_db"]),
		FadeInMs:       fields["fade_in_ms"],
		FadeOutMs:      fields["fade_out_ms"],
		Reverse:        fields["reverse"] >= 0.5,
		Normalize:      fields["normalize"] >= 0.5,
	}
}

// SampleEditSource is the load-side persistence view (userprefs
// SampleEditStore satisfies it). Mirrors UserRecipeSource / SampleSource.
type SampleEditSource interface {
	LoadSampleEdits() (map[string]map[string]float64, error)
}

// ApplySavedSampleEdits rehydrates persisted descriptors at startup via
// SetSampleEdit, which also pushes each one over the platform bridge so the
// browser render cache is correct from the first play. Call after
// bindBuiltinInstrumentRecipes so the recipe bindings the descriptors decorate
// are in place. Nil source / load errors are non-fatal no-ops.
func ApplySavedSampleEdits(src SampleEditSource) {
	if src == nil {
		return
	}
	edits, err := src.LoadSampleEdits()
	if err != nil {
		return
	}
	for id, fields := range edits {
		if id == "" || len(fields) == 0 {
			continue
		}
		SetSampleEdit(id, SampleEditFromFields(fields))
	}
}

// ApplySampleEditToBuffer renders e against a freshly-rendered recipe buffer.
// It is a thin wrapper over BakeSample so the Sampler tab's bake path and the
// voice dispatcher share ONE DSP implementation.
func ApplySampleEditToBuffer(buf []float32, sampleRate int, e SampleEdit) []float32 {
	return BakeSample(buf, sampleRate, e)
}
