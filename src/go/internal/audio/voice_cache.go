//go:build !test && !js

package audio

import (
	"sync"
	"sync/atomic"
)

// RoundRobinVariants is the number of pre-rendered voice variants per cache
// key. The C synth uses ma_noise with different seeds per render call, so
// rendering the same function N times produces subtly different buffers.
// This eliminates the "machine gun effect" on rapid repeated hits.
const RoundRobinVariants = 3

// voiceCacheKey uniquely identifies a rendered voice buffer. Core instruments
// (snare, kick, etc.) use fixed DurationSec from config — they do NOT depend
// on BPM, so bpm is 0 for them. Variant instruments use bpm for duration, so
// bpm is included. WAV samples are pre-loaded and bypass the cache entirely.
// SynthParams are included so different parameter sets produce different cache
// entries (e.g., pitch-shifted snare vs default snare).
type voiceCacheKey struct {
	instrumentID string
	bpm          int // 0 for core instruments that don't depend on BPM
	sampleRate   int
	synthParams  SynthParams // zero value = default params
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
	mu      sync.RWMutex
	entries map[voiceCacheKey]*roundRobinEntry
	maxSize int // cap number of entries to limit memory
}

var globalVoiceCache = &voiceCache{
	entries: make(map[voiceCacheKey]*roundRobinEntry),
	maxSize: 256,
}

// Get returns a CLONE of a round-robin selected cached buffer.
// Each voice needs its own copy because cVoice tracks playback position.
func (vc *voiceCache) Get(key voiceCacheKey) ([]float32, bool) {
	vc.mu.RLock()
	entry, ok := vc.entries[key]
	vc.mu.RUnlock()
	if !ok || len(entry.variants) == 0 {
		return nil, false
	}
	// Round-robin: select next variant.
	idx := entry.counter.Add(1) % uint64(len(entry.variants))
	buf := entry.variants[idx]
	clone := make([]float32, len(buf))
	copy(clone, buf)
	return clone, true
}

// Put stores a copy of buf as one variant in the cache. Accumulates up to
// RoundRobinVariants before the entry is considered "full".
func (vc *voiceCache) Put(key voiceCacheKey, buf []float32) {
	stored := make([]float32, len(buf))
	copy(stored, buf)

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
		entry.variants = append(entry.variants, stored)
	}
	vc.mu.Unlock()
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
