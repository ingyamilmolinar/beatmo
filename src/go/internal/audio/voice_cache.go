//go:build !test && !js

package audio

import (
	"sync"
	"sync/atomic"
)

// RoundRobinVariants is the number of pre-rendered voice variants per cache
// key. NOTE: the C renderers seed ma_noise with 0 (→ 4321 inside
// ma_noise_config_init) on every call, so consecutive renders are
// byte-identical and the variants do NOT actually differ today (pinned by
// TestLegacyRendersAreDeterministicAcrossCalls). The slot machinery is kept
// for a future per-variant seed input (modular noise_seed already supports
// one).
const RoundRobinVariants = 3

// voiceCacheKey uniquely identifies a rendered voice buffer. Core instruments
// (snare, kick, etc.) use fixed DurationSec from config — they do NOT depend
// on BPM, so bpm is 0 for them. Variant instruments use bpm for duration, so
// bpm is included. WAV samples are pre-loaded and bypass the cache entirely.
// paramsHash is a deterministic FNV-1a fingerprint of the RecipeParams map
// (see hashRecipeParams in synth_recipe.go); the zero value means "default
// params" so the cache key is byte-identical for all-zero SynthParams and
// for instruments that don't carry recipe params at all.
//
// pitch is the node semitone offset used by pitch-aware melodic recipes
// (pitchAwareRecipe). For drums and non-pitch-aware instruments pitch is
// always 0, so their keys are unchanged — drum cache + goldens stay
// byte-identical. For melodic synths pitch is the rounded node semitone
// value so each pitch level gets its own rendered-at-pitch buffer.
type voiceCacheKey struct {
	instrumentID string
	bpm          int // 0 for core instruments that don't depend on BPM
	sampleRate   int
	paramsHash   uint64
	pitch        float64 // 0 for non-pitch-aware; rounded semitones for melodic
}

// roundRobinEntry stores N variants of a rendered voice buffer plus a
// round-robin counter for cycling through them.
type roundRobinEntry struct {
	variants [][]float32 // N pre-rendered buffers
	counter  atomic.Uint64
}

// voiceCache stores rendered voice buffers keyed by instrument+params.
// Uses round-robin selection across N variants to avoid the "machine gun
// effect" when the same instrument fires repeatedly.
type voiceCache struct {
	mu          sync.RWMutex
	entries     map[voiceCacheKey]*roundRobinEntry
	maxSize     int                  // cap number of entries to limit memory
	lastSamples map[string][]float32 // most recently Put'd buffer per instrumentID (Phase 4)
}

var globalVoiceCache = &voiceCache{
	entries:     make(map[voiceCacheKey]*roundRobinEntry),
	maxSize:     256,
	lastSamples: make(map[string][]float32),
}

// Get returns a SHARED (not cloned) round-robin-selected cached buffer.
// Playback position is tracked outside the buffer in cVoice.i (see
// drums_c.go) — Sample/SampleBlock never write to v.buf, so multiple
// concurrent voices reading the same backing array is safe. The clone
// removed here saved ~80 KB per cache-hit trigger; together with the
// (former) tryRecipeVoice clone it removes the per-trigger 160 KB copy
// path that the synth-tab profile attributed to PlayParams.
func (vc *voiceCache) Get(key voiceCacheKey) ([]float32, bool) {
	vc.mu.RLock()
	entry, ok := vc.entries[key]
	vc.mu.RUnlock()
	if !ok || len(entry.variants) == 0 {
		return nil, false
	}
	// Round-robin: select next variant.
	idx := entry.counter.Add(1) % uint64(len(entry.variants))
	return entry.variants[idx], true
}

// Put stores buf as one variant in the cache. Accumulates up to
// RoundRobinVariants before the entry is considered "full".
//
// CONTRACT: callers (tryRecipeVoice, CVariantInstrument render paths)
// must not mutate buf after calling Put — the cache and any concurrent
// Get reader share the backing array. The clone removed here saved
// ~80 KB per cache-miss trigger; per-trigger ownership transfer is now
// implicit at the Put boundary.
func (vc *voiceCache) Put(key voiceCacheKey, buf []float32) {
	vc.mu.Lock()
	entry, ok := vc.entries[key]
	if !ok {
		if len(vc.entries) >= vc.maxSize {
			vc.mu.Unlock()
			return
		}
		entry = &roundRobinEntry{}
		vc.entries[key] = entry
	}
	if len(entry.variants) < RoundRobinVariants {
		entry.variants = append(entry.variants, buf)
	}
	// Track the most recent buffer per instrumentID for the UI's
	// Synth-tab waveform preview (Phase 4). Shares the backing array
	// per the Put contract — readers must treat it as read-only.
	if vc.lastSamples == nil {
		vc.lastSamples = make(map[string][]float32)
	}
	vc.lastSamples[key.instrumentID] = buf
	vc.mu.Unlock()
}

// LatestSample returns the most recently Put-cached buffer for the
// given instrument, or nil if the instrument has never been rendered.
// The slice shares the cache's backing array — read-only. Phase 4.
func (vc *voiceCache) LatestSample(instrumentID string) []float32 {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.lastSamples[instrumentID]
}

// LatestVoiceSample is the package-level accessor the UI calls to
// render the Synth-tab cached waveform preview. Returns nil if the
// instrument has never been rendered. Native build only — the stub
// in voice_cache_stub.go returns nil unconditionally.
func LatestVoiceSample(instrumentID string) []float32 {
	return globalVoiceCache.LatestSample(instrumentID)
}

// IsFull returns true if the cache entry for key has all N variants rendered.
func (vc *voiceCache) IsFull(key voiceCacheKey) bool {
	vc.mu.RLock()
	entry, ok := vc.entries[key]
	vc.mu.RUnlock()
	if !ok {
		return false
	}
	return len(entry.variants) >= RoundRobinVariants
}

// Clear removes all cached entries. Called on instrument reset.
func (vc *voiceCache) Clear() {
	vc.mu.Lock()
	vc.entries = make(map[voiceCacheKey]*roundRobinEntry)
	vc.mu.Unlock()
}

// ClearInstrument removes every cache entry for a single instrument id.
// Used by SetInstrumentParam / SetInstrumentParams in synth_recipe.go so
// param edits invalidate stale buffers without touching unrelated entries.
func (vc *voiceCache) ClearInstrument(instrumentID string) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	for k := range vc.entries {
		if k.instrumentID == instrumentID {
			delete(vc.entries, k)
		}
	}
}

func init() {
	// Wire the manager's invalidation hook to the native voice cache so
	// per-instrument param edits drop stale buffers on the next trigger.
	voiceCacheInvalidate = globalVoiceCache.ClearInstrument
}

// VoiceCacheStatsForTest returns the live entry count and approximate retained
// bytes across all variants in the native voice cache. Soak tests use this to
// detect runaway key churn (SetInstrumentParam invalidates one instrument id
// but a paramsHash that changes every tick can still rotate keys faster than
// the cap protects against the underlying memory footprint).
func VoiceCacheStatsForTest() (entries int, totalBytes int64) {
	globalVoiceCache.mu.RLock()
	defer globalVoiceCache.mu.RUnlock()
	for _, e := range globalVoiceCache.entries {
		entries++
		for _, v := range e.variants {
			totalBytes += int64(len(v)) * 4
		}
	}
	return
}
