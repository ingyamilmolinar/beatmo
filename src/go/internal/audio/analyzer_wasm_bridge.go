//go:build js && wasm

package audio

import (
	"syscall/js"
	"unsafe"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Keep the per-Draw fetch rate bounded by snapshotCacheTTL even at 120+ fps.
var (
	channelSnapCache = newSnapshotCache()
	preEQSnapCache   = newSnapshotCache()
	synthSnapCache   = newSnapshotCache()
)

// For tests that observe back-to-back fetches without TTL interference.
func SnapshotCacheBypassForTest() {
	channelSnapCache.mu.Lock()
	channelSnapCache.entries = make(map[channel.ID]*snapshotCacheEntry)
	channelSnapCache.mu.Unlock()
	preEQSnapCache.mu.Lock()
	preEQSnapCache.entries = make(map[channel.ID]*snapshotCacheEntry)
	preEQSnapCache.mu.Unlock()
	synthSnapCache.mu.Lock()
	synthSnapCache.entries = make(map[channel.ID]*snapshotCacheEntry)
	synthSnapCache.mu.Unlock()
}

// One bulk js.CopyBytesToGo instead of a js.Value per element. nil when the length field is absent (a JS bundle
// without the typed array) or zero.
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
	// Safe: the Float32Array and Go's float32 share the little-endian layout on wasm.
	f32 := unsafe.Slice((*float32)(unsafe.Pointer(&scratch[0])), n)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = float64(f32[i])
	}
	return out
}

// The fallback for a JS bundle without the typed array: N js.Value allocations per call, the pattern behind the
// WASM OOM.
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

func readSnapshotF64(val js.Value, typedKey, lenKey, legacyKey string) []float64 {
	if out := readSnapshotF64FromTypedArray(val, typedKey, lenKey); out != nil {
		return out
	}
	return readSnapshotF64Legacy(val, legacyKey)
}
