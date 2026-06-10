//go:build js && wasm

package audio

import (
	"sync"
	"syscall/js"
	"time"
	"unsafe"
)

// Per-tap snapshot caches. The bridge dispatcher routes ChannelAnalyzer-
// Snapshot / PreEQAnalyzerSnapshot / SynthAnalyzerSnapshot lookups
// through these so the per-Draw fetch rate stays bounded by
// snapshotCacheTTL even when the UI runs at 120+ fps.
var (
	channelSnapCache = newSnapshotCache()
	preEQSnapCache   = newSnapshotCache()
	synthSnapCache   = newSnapshotCache()
	sendBusSnap      = snapshotCacheEntry{}
	sendBusSnapMu    sync.Mutex
)

// SnapshotCacheBypassForTest unconditionally clears the per-snapshot
// caches. Used by tests that need to observe back-to-back fetches
// without TTL interference.
func SnapshotCacheBypassForTest() {
	channelSnapCache.mu.Lock()
	channelSnapCache.entries = make(map[string]*snapshotCacheEntry)
	channelSnapCache.mu.Unlock()
	preEQSnapCache.mu.Lock()
	preEQSnapCache.entries = make(map[string]*snapshotCacheEntry)
	preEQSnapCache.mu.Unlock()
	synthSnapCache.mu.Lock()
	synthSnapCache.entries = make(map[string]*snapshotCacheEntry)
	synthSnapCache.mu.Unlock()
	sendBusSnapMu.Lock()
	sendBusSnap = snapshotCacheEntry{}
	sendBusSnapMu.Unlock()
}

// readSnapshotF64FromTypedArray reads a Float32Array view (exposed as a
// Uint8Array on the JS side under bufKey) into a freshly-allocated
// []float64 of length lenVal.Int(). Uses js.CopyBytesToGo for a single
// bulk transfer — one js.Value for the typed array plus one for the
// length scalar, instead of N per-element v.Index(i).Float() calls.
//
// Returns nil when either the length field is absent (legacy JS bundle
// without the typed-array attachment) or the length is zero.
func readSnapshotF64FromTypedArray(val js.Value, bufKey, lenKey string) []float64 {
	lenVal := val.Get(lenKey)
	if !lenVal.Truthy() {
		return nil
	}
	n := lenVal.Int()
	if n <= 0 {
		return nil
	}
	u8 := val.Get(bufKey)
	if !u8.Truthy() {
		return nil
	}
	nBytes := n * 4 // sizeof(float32)
	scratch := getSnapshotByteBuf(nBytes)
	defer putSnapshotByteBuf(scratch)
	js.CopyBytesToGo(scratch, u8)
	bridgeSnapshotElementReads.Add(uint64(n))
	// Reinterpret []byte as []float32 (same backing array). Safe because
	// the Float32Array we got from JS has the same little-endian wire
	// representation as Go's float32 on the wasm target.
	f32 := unsafe.Slice((*float32)(unsafe.Pointer(&scratch[0])), n)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = float64(f32[i])
	}
	return out
}

// readSnapshotF64Legacy reads a plain JS Array under fieldKey via the
// per-element v.Index(i).Float() loop. Used as the fallback when the JS
// side hasn't been rebuilt with the typed-array attachment. This is the
// original implementation; calling it costs N js.Value allocations and
// is the pattern that drove the production WASM OOM.
func readSnapshotF64Legacy(val js.Value, fieldKey string) []float64 {
	v := val.Get(fieldKey)
	if !v.Truthy() || v.Length() == 0 {
		return nil
	}
	n := v.Length()
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = v.Index(i).Float()
	}
	bridgeSnapshotElementReads.Add(uint64(n))
	return out
}

// readSnapshotF64 prefers the typed-array fast path and falls back to
// the legacy per-element loop only when the JS bundle predates the
// typed-array attachment. typedKey is the U8 view field name
// (e.g. "spectrumU8"); legacyKey is the old plain-array field name
// (e.g. "spectrum").
func readSnapshotF64(val js.Value, typedKey, lenKey, legacyKey string) []float64 {
	if out := readSnapshotF64FromTypedArray(val, typedKey, lenKey); out != nil {
		return out
	}
	return readSnapshotF64Legacy(val, legacyKey)
}

// sendBusSnapshotCachedOrFetch wraps the dispatcher used by
// SendBusAnalyzerSnapshot — the send bus is a single shared analyser
// (no id keying) so it has its own scalar cache entry rather than a
// map-backed snapshotCache.
func sendBusSnapshotCachedOrFetch(fetch func() AnalyzerSnapshot) AnalyzerSnapshot {
	sendBusSnapMu.Lock()
	now := time.Now()
	if !sendBusSnap.lastFetch.IsZero() && now.Sub(sendBusSnap.lastFetch) < snapshotCacheTTL {
		snap := sendBusSnap.snap
		sendBusSnapMu.Unlock()
		return snap
	}
	sendBusSnapMu.Unlock()

	snap := fetch()
	sendBusSnapMu.Lock()
	sendBusSnap = snapshotCacheEntry{lastFetch: time.Now(), snap: snap}
	sendBusSnapMu.Unlock()
	return snap
}
