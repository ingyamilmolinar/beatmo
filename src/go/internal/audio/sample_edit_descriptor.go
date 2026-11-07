package audio

import (
	"math"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// A descriptor keeps a synth instrument a SYNTH: the edit is applied to the fresh render at trigger time. Its hash is
// folded into the voice-cache key, so an edit ROTATES the key rather than invalidating: dropping every pitch's entry
// made each following note cold-render on the dispatch goroutine.

var (
	sampleEditsMu sync.RWMutex
	sampleEdits   = map[instrument.ID]SampleEdit{}
)

// So the browser bridge can sync; Clear pushes the identity edit so JS deletes its entry.
var platformSampleEditChanged = func(id instrument.ID, e SampleEdit) {}

func SwapPlatformSampleEditChangedForTest(fn func(id instrument.ID, e SampleEdit)) func(id instrument.ID, e SampleEdit) {
	prev := platformSampleEditChanged
	if fn != nil {
		platformSampleEditChanged = fn
	}
	return prev
}

// AUDIBLE without announcing: every live Sampler control writes through this, since sound must not wait for the
// gesture to end. The UI's commit site publishes once; SetSampleEdit is this plus the publish, for callers that ARE
// the gesture (import, the JS bridge, undo replay).
func ApplySampleEditLive(instID instrument.ID, e SampleEdit) bool {
	if e.isIdentity() {
		return clearSampleEditLive(instID)
	}
	sampleEditsMu.Lock()
	// A full re-import repeats every edit on undo/redo; re-rendering for nothing is skipped.
	if cur, ok := sampleEdits[instID]; ok && cur == e {
		sampleEditsMu.Unlock()
		return false
	}
	sampleEdits[instID] = e
	bumpVoiceRev(instID)
	sampleEditsMu.Unlock()

	// No cache invalidation: the edit's hash already rotates the key. User samples are re-derived instead, which is not
	// cache-keyed.
	reapplyUserSampleEdit(instID, e)
	platformSampleEditChanged(instID, e)
	return true
}

// Publishing per detent would log a line per notch: a turned control uses ApplySampleEditLive.
func SetSampleEdit(instID instrument.ID, e SampleEdit) {
	if e.isIdentity() {
		ClearSampleEdit(instID)
		return
	}
	if !ApplySampleEditLive(instID, e) {
		return
	}
	hooks.PublishKind(hooks.EventSampleEditChanged, hooks.SamplePayload{SampleID: instID})
}

// A user sample has no synth to re-render, so the playable PCM is re-derived from its PRISTINE source. From the CURRENT
// pristine, not the first-loaded one: import puts the embedded PCM and THEN sets the edit.
func reapplyUserSampleEdit(id instrument.ID, e SampleEdit) {
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

// A no-op when none exists.
func ClearSampleEdit(instID instrument.ID) {
	if clearSampleEditLive(instID) {
		hooks.PublishKind(hooks.EventSampleEditChanged, hooks.SamplePayload{SampleID: instID})
	}
}

// A control turned back to neutral must be audible at once like any other value.
func clearSampleEditLive(instID instrument.ID) bool {
	sampleEditsMu.Lock()
	_, had := sampleEdits[instID]
	delete(sampleEdits, instID)
	sampleEditsMu.Unlock()
	if !had {
		return false
	}
	bumpVoiceRev(instID)
	// No cache invalidation: the key rotates by itself. Restores a user sample's pristine playback.
	reapplyUserSampleEdit(instID, SampleEdit{StartFrac: 0, EndFrac: 1})
	platformSampleEditChanged(instID, SampleEdit{EndFrac: 1}) // identity ⇒ JS deletes its entry
	return true
}

func SampleEditFor(instID instrument.ID) (SampleEdit, bool) {
	sampleEditsMu.RLock()
	defer sampleEditsMu.RUnlock()
	e, ok := sampleEdits[instID]
	return e, ok
}

func HasSampleEdit(instID instrument.ID) bool {
	sampleEditsMu.RLock()
	defer sampleEditsMu.RUnlock()
	_, ok := sampleEdits[instID]
	return ok
}

// Folded into paramsHash, so a descriptor change misses the cache like a param change.
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
	var flags uint64
	if e.Reverse {
		flags |= 1
	}
	if e.Normalize {
		flags |= 2
	}
	if e.Fade {
		flags |= 4
	}
	mix(flags)
	return h
}

// 0 when none; lets the Sampler re-sync after an external change (import, rehydration).
func SampleEditSignature(instID instrument.ID) uint64 {
	e, ok := SampleEditFor(instID)
	if !ok {
		return 0
	}
	return hashSampleEdit(e)
}

// Bools as 0/1. The audio package owns the shape userprefs persists.
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
		"fade":            b2f(e.Fade),
		"reverse":         b2f(e.Reverse),
		"normalize":       b2f(e.Normalize),
	}
}

// Unknown keys are ignored; a missing EndFrac bakes an empty buffer, audibly wrong but never unsafe.
func SampleEditFromFields(fields map[string]float64) SampleEdit {
	return SampleEdit{
		StartFrac:      fields["start_frac"],
		EndFrac:        fields["end_frac"],
		TransposeSemis: fields["transpose_semis"],
		DetuneCents:    fields["detune_cents"],
		GainDB:         float32(fields["gain_db"]),
		Fade:           fields["fade"] >= 0.5,
		Reverse:        fields["reverse"] >= 0.5,
		Normalize:      fields["normalize"] >= 0.5,
	}
}

// Edits saved before the versioned library; read once into the first versions.
type SampleEditSource interface {
	LoadSampleEdits() (map[instrument.ID]map[string]float64, error)
}

// The Sampler's bake and the dispatcher share ONE DSP implementation.
func ApplySampleEditToBuffer(buf []float32, sampleRate int, e SampleEdit) []float32 {
	return BakeSample(buf, sampleRate, e)
}
