//go:build !test && !js

package audio

import (
	"sync"
	"sync/atomic"
	"time"
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
	bytes    int64        // sum of len(variant)*4 across variants, for the byte budget
	lastNS   atomic.Int64 // last access (Get/Put) UnixNano, for LRU eviction
}

// defaultVoiceCacheBudgetBytes is the default retained-buffer ceiling. 48 MB is
// tighter than the old 256-entry cap's ~135 MB worst case (256 × 3 × 176 KB
// organ variants), yet ample for the demo working set + predictive
// neighborhood. Overridable via SetVoiceCacheBudgetBytes (RuntimeProfile wires
// a per-platform value; browser sets a smaller one).
const defaultVoiceCacheBudgetBytes int64 = 48 << 20

// defaultVoiceCacheMaxEntries bounds the map size + eviction-scan cost
// regardless of buffer sizes (a secondary guard behind the byte budget).
const defaultVoiceCacheMaxEntries = 1024

// voiceCache stores rendered voice buffers keyed by instrument+params.
// Uses round-robin selection across N variants to avoid the "machine gun
// effect" when the same instrument fires repeatedly. Memory is bounded by a
// hard byte budget with LRU-by-access eviction (predictive neighborhood warming
// can mint many keys, so an entry-count cap is not a safe memory proxy).
type voiceCache struct {
	mu          sync.RWMutex
	entries     map[voiceCacheKey]*roundRobinEntry
	maxEntries  int                  // secondary guard: bounds map size + scan cost
	budgetBytes int64                // primary memory ceiling (retained buffer bytes)
	curBytes    int64                // current retained bytes (sum of entry.bytes)
	lastSamples map[string][]float32 // most recently Put'd buffer per instrumentID (Phase 4)
}

var globalVoiceCache = &voiceCache{
	entries:     make(map[voiceCacheKey]*roundRobinEntry),
	maxEntries:  defaultVoiceCacheMaxEntries,
	budgetBytes: defaultVoiceCacheBudgetBytes,
	lastSamples: make(map[string][]float32),
}

// SetVoiceCacheBudgetBytes sets the retained-buffer ceiling (0 disables the
// byte budget, leaving only the entry-count guard). Evicts immediately if the
// new budget is below current usage. Called once at startup from the UI runtime
// profile; also used by tests/benches to exercise eviction with a tiny budget.
func SetVoiceCacheBudgetBytes(n int64) {
	globalVoiceCache.mu.Lock()
	globalVoiceCache.budgetBytes = n
	globalVoiceCache.evictToBudgetLocked(voiceCacheKey{})
	globalVoiceCache.mu.Unlock()
}

// nowNS is the LRU clock. A package var so tests can make eviction order
// deterministic without a real wall clock.
var nowNS = func() int64 { return time.Now().UnixNano() }

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
	// LRU stamp: a played/probed voice is "recent" and thus protected from
	// eviction. Atomic store is safe under the RLock — no Get serialization.
	entry.lastNS.Store(nowNS())
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
	nb := int64(len(buf)) * 4
	vc.mu.Lock()
	entry, ok := vc.entries[key]
	if !ok {
		// Secondary entry-count guard: evict the LRU entry to make room rather
		// than dropping the new one, so the freshest render always lands.
		if vc.maxEntries > 0 && len(vc.entries) >= vc.maxEntries {
			vc.evictLRULocked(key)
		}
		entry = &roundRobinEntry{}
		vc.entries[key] = entry
	}
	if len(entry.variants) < RoundRobinVariants {
		entry.variants = append(entry.variants, buf)
		entry.bytes += nb
		vc.curBytes += nb
	}
	entry.lastNS.Store(nowNS())
	// Track the most recent buffer per instrumentID for the UI's
	// Synth-tab waveform preview (Phase 4). Shares the backing array
	// per the Put contract — readers must treat it as read-only.
	if vc.lastSamples == nil {
		vc.lastSamples = make(map[string][]float32)
	}
	vc.lastSamples[key.instrumentID] = buf
	// Enforce the byte ceiling, never evicting the entry we just wrote (it has
	// the newest lastNS anyway, so it is never the LRU victim).
	vc.evictToBudgetLocked(key)
	vc.mu.Unlock()
}

// evictLRULocked removes the single least-recently-accessed entry other than
// keep. Caller holds vc.mu. No-op if only keep remains.
func (vc *voiceCache) evictLRULocked(keep voiceCacheKey) {
	var victim voiceCacheKey
	var oldest int64
	found := false
	for k, e := range vc.entries {
		if k == keep {
			continue
		}
		if ln := e.lastNS.Load(); !found || ln < oldest {
			oldest, victim, found = ln, k, true
		}
	}
	if !found {
		return
	}
	vc.curBytes -= vc.entries[victim].bytes
	if vc.curBytes < 0 {
		vc.curBytes = 0
	}
	delete(vc.entries, victim)
}

// evictToBudgetLocked evicts LRU entries until curBytes <= budgetBytes (0
// budget disables it). Never evicts keep. Caller holds vc.mu. Deleting a map
// entry whose buffer a live cVoice still reads is safe — Go GC keeps the slice
// alive until that voice finishes; only the lookup is dropped.
func (vc *voiceCache) evictToBudgetLocked(keep voiceCacheKey) {
	if vc.budgetBytes <= 0 {
		return
	}
	for vc.curBytes > vc.budgetBytes && len(vc.entries) > 1 {
		before := len(vc.entries)
		vc.evictLRULocked(keep)
		if len(vc.entries) == before {
			break // only keep remains over budget — can't shrink further
		}
	}
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

// Has reports whether key has at least one rendered variant, without mutating
// the round-robin counter (unlike Get). Used by VoiceCached for a side-effect-
// free "is this pitch already warm?" probe from the UI prewarmer.
func (vc *voiceCache) Has(key voiceCacheKey) bool {
	vc.mu.RLock()
	entry, ok := vc.entries[key]
	vc.mu.RUnlock()
	return ok && len(entry.variants) > 0
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
	vc.curBytes = 0
	vc.mu.Unlock()
	// "" => whole-cache invalidation, so the prewarmer re-warms everything.
	notifyVoiceCacheInvalidated("")
}

// ClearInstrument removes every cache entry for a single instrument id.
// Used by SetInstrumentParam / SetInstrumentParams in synth_recipe.go so
// param edits invalidate stale buffers without touching unrelated entries.
func (vc *voiceCache) ClearInstrument(instrumentID string) {
	vc.mu.Lock()
	for k, e := range vc.entries {
		if k.instrumentID == instrumentID {
			vc.curBytes -= e.bytes
			delete(vc.entries, k)
		}
	}
	if vc.curBytes < 0 {
		vc.curBytes = 0
	}
	vc.mu.Unlock()
	notifyVoiceCacheInvalidated(instrumentID)
}

func init() {
	// Wire the manager's invalidation hook to the native voice cache so
	// per-instrument param edits drop stale buffers on the next trigger.
	voiceCacheInvalidate = globalVoiceCache.ClearInstrument
}

// ClearVoiceCacheForTest drops every cached voice. Exported for benchmarks that
// must start from an identical cold cache: the cache is process-global, so
// without this a scenario's numbers depend on whatever ran before it (measured:
// the same baseline scenario reported 2 / 60 / 51 triggers across runs, and a
// fully-warm run reported ZERO renders).
func ClearVoiceCacheForTest() { globalVoiceCache.Clear() }

// getAnyForInstrumentPitch returns any cached buffer for (instrumentID, pitch)
// under ANY paramsHash, preferring the most-recently-used. This is the
// best-available fallback source: when the exact binding is not rendered yet,
// an older-params render of the SAME note at the SAME pitch is far closer to
// correct than either blocking (a late beat) or silence (a dropped beat).
func (vc *voiceCache) getAnyForInstrumentPitch(id string, pitch float64) ([]float32, bool) {
	rounded := roundPitchForCache(pitch)
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	var best *roundRobinEntry
	var bestNS int64
	for k, e := range vc.entries {
		if k.instrumentID != id || k.pitch != rounded || len(e.variants) == 0 {
			continue
		}
		if ln := e.lastNS.Load(); best == nil || ln > bestNS {
			best, bestNS = e, ln
		}
	}
	if best == nil {
		return nil, false
	}
	idx := best.counter.Add(1) % uint64(len(best.variants))
	return best.variants[idx], true
}

// getAnyForInstrument returns any cached buffer for instrumentID regardless of
// pitch or paramsHash — the last-resort fallback when even the target pitch has
// never been rendered (a genuinely new pitch appearing for the first time, so
// getAnyForInstrumentPitch also misses). Playing an existing render at the
// wrong pitch ON TIME is judged better than a synchronous ~24ms CGo render on
// the audioLoop dispatch goroutine (invariant I3): the caller's async submit
// still targets the correct pitch, so the mismatch self-corrects on the very
// next trigger. Only reached once per (instrument, never-seen-pitch) — every
// subsequent trigger at that pitch hits getAnyForInstrumentPitch instead.
func (vc *voiceCache) getAnyForInstrument(id string) ([]float32, bool) {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	var best *roundRobinEntry
	var bestNS int64
	for k, e := range vc.entries {
		if k.instrumentID != id || len(e.variants) == 0 {
			continue
		}
		if ln := e.lastNS.Load(); best == nil || ln > bestNS {
			best, bestNS = e, ln
		}
	}
	if best == nil {
		return nil, false
	}
	idx := best.counter.Add(1) % uint64(len(best.variants))
	return best.variants[idx], true
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
