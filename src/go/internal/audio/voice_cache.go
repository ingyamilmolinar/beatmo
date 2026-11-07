//go:build !test && !js

package audio

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The C renderers seed their noise identically every call, so the variants do not differ yet; the slots stay for a
// per-variant seed (noise_seed already supports one).
const RoundRobinVariants = 3

// No tempo: a voice is render.seconds long, so a BPM change re-renders nothing. paramsHash 0 = no document. pitch is the
// rounded node semitone for melodic instruments and always 0 otherwise, so drum keys stay byte-identical.
type voiceCacheKey struct {
	instrumentID instrument.ID
	sampleRate   int
	paramsHash   uint64
	pitch        float64 // 0 for non-pitch-aware; rounded semitones for melodic
}

type roundRobinEntry struct {
	variants [][]float32 // N pre-rendered buffers
	counter  atomic.Uint64
	bytes    int64        // sum of len(variant)*4 across variants, for the byte budget
	lastNS   atomic.Int64 // last access (Get/Put) UnixNano, for LRU eviction
}

// Ample for the demo working set plus the predictive neighbourhood; the runtime profile sets a per-platform value.
const defaultVoiceCacheBudgetBytes int64 = 48 << 20

// Bounds map size and eviction-scan cost whatever the buffer sizes.
const defaultVoiceCacheMaxEntries = 1024

// Round-robin across variants avoids the machine-gun effect. A hard byte budget with LRU eviction, since predictive
// warming mints many keys and an entry count is no memory proxy.
type voiceCache struct {
	mu          sync.RWMutex
	entries     map[voiceCacheKey]*roundRobinEntry
	maxEntries  int                         // secondary guard: bounds map size + scan cost
	budgetBytes int64                       // primary memory ceiling (retained buffer bytes)
	curBytes    int64                       // current retained bytes (sum of entry.bytes)
	lastSamples map[instrument.ID][]float32 // most recently Put buffer per instrument
}

var globalVoiceCache = &voiceCache{
	entries:     make(map[voiceCacheKey]*roundRobinEntry),
	maxEntries:  defaultVoiceCacheMaxEntries,
	budgetBytes: defaultVoiceCacheBudgetBytes,
	lastSamples: make(map[instrument.ID][]float32),
}

// 0 disables the byte budget. Evicts at once if below current usage.
func SetVoiceCacheBudgetBytes(n int64) {
	globalVoiceCache.mu.Lock()
	globalVoiceCache.budgetBytes = n
	globalVoiceCache.evictToBudgetLocked(voiceCacheKey{})
	globalVoiceCache.mu.Unlock()
}

// A var so tests can make eviction order deterministic.
var nowNS = func() int64 { return time.Now().UnixNano() }

// SHARED, not cloned: playback position lives in cVoice.i and Sample never writes the buffer, so concurrent voices can
// read one array.
func (vc *voiceCache) Get(key voiceCacheKey) ([]float32, bool) {
	vc.mu.RLock()
	entry, ok := vc.entries[key]
	vc.mu.RUnlock()
	if !ok || len(entry.variants) == 0 {
		return nil, false
	}
	// Atomic, so it is safe under the RLock without serializing Get.
	entry.lastNS.Store(nowNS())
	// Round-robin: select next variant.
	idx := entry.counter.Add(1) % uint64(len(entry.variants))
	return entry.variants[idx], true
}

// CONTRACT: callers must not mutate buf after Put; the cache and concurrent readers share the array.
func (vc *voiceCache) Put(key voiceCacheKey, buf []float32) {
	nb := int64(len(buf)) * 4
	vc.mu.Lock()
	entry, ok := vc.entries[key]
	if !ok {
		// Evict the LRU entry rather than drop the new one, so the freshest render always lands.
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
	// Shares the backing array: read-only.
	if vc.lastSamples == nil {
		vc.lastSamples = make(map[instrument.ID][]float32)
	}
	vc.lastSamples[key.instrumentID] = buf
	// The entry just written has the newest lastNS, so it is never the victim.
	vc.evictToBudgetLocked(key)
	vc.mu.Unlock()
}

// Caller holds vc.mu.
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

// Caller holds vc.mu. A live cVoice keeps an evicted buffer alive through the GC; only the lookup goes.
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

// Read-only: shares the cache's array.
func (vc *voiceCache) LatestSample(instrumentID instrument.ID) []float32 {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.lastSamples[instrumentID]
}

// nil if never rendered; nil on the stub build.
func LatestVoiceSample(instrumentID instrument.ID) []float32 {
	return globalVoiceCache.LatestSample(instrumentID)
}

// Without touching the round-robin counter: a side-effect-free warmth probe.
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

// Without touching unrelated entries.
func (vc *voiceCache) ClearInstrument(instrumentID instrument.ID) {
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
	voiceCacheInvalidate = globalVoiceCache.ClearInstrument
}

// The cache is process-global, so without this a benchmark's numbers depend on what ran before it.
func ClearVoiceCacheForTest() { globalVoiceCache.Clear() }

// The best-available fallback: an older render of the SAME note is far closer than a late beat or silence.
func (vc *voiceCache) getAnyForInstrumentPitch(id instrument.ID, pitch float64) ([]float32, bool) {
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

// The last resort for a never-rendered pitch: the wrong pitch ON TIME beats a synchronous render on the dispatch
// goroutine, and the async submit corrects the next trigger.
func (vc *voiceCache) getAnyForInstrument(id instrument.ID) ([]float32, bool) {
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

// Soak tests catch runaway key churn with it.
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
