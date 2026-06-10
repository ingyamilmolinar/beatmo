//go:build js

package userprefs

import (
	"sync"
	"syscall/js"
)

// Browser SampleStore backend. PCM payloads (MBs) exceed localStorage's ~5 MB
// cap, so samples live in IndexedDB via a small JS shim (idbGetAllSamples /
// idbPutSample / idbDeleteSample in audio.js). IndexedDB is async; the Go
// SampleStore surface is synchronous, so we keep an in-memory cache hydrated
// once from IndexedDB (blocking on the read Promise via a channel, the same
// pattern audio.SelectWAV uses) and write through on Save/Delete. Hydration
// happens on the first LoadSamples — which the bootstrap calls (through
// audio.ApplySavedSamples) before the first Layout, so the picker is populated
// in time.
var (
	sampleCacheMu  sync.RWMutex
	sampleCache    = map[string]SampleBlob{}
	sampleHydrated bool
)

func (s *localStorageStore) LoadSamples() (map[string]SampleBlob, error) {
	hydrateSamplesOnce()
	sampleCacheMu.RLock()
	defer sampleCacheMu.RUnlock()
	out := make(map[string]SampleBlob, len(sampleCache))
	for id, b := range sampleCache {
		out[id] = SampleBlob{SampleRate: b.SampleRate, PCM: append([]byte(nil), b.PCM...)}
	}
	return out, nil
}

func (s *localStorageStore) SaveSample(id string, blob SampleBlob) error {
	if id == "" {
		return nil
	}
	sampleCacheMu.Lock()
	sampleCache[id] = SampleBlob{SampleRate: blob.SampleRate, PCM: append([]byte(nil), blob.PCM...)}
	sampleCacheMu.Unlock()
	if fn := js.Global().Get("idbPutSample"); fn.Truthy() {
		u8 := js.Global().Get("Uint8Array").New(len(blob.PCM))
		if len(blob.PCM) > 0 {
			js.CopyBytesToJS(u8, blob.PCM)
		}
		fn.Invoke(id, blob.SampleRate, u8)
	}
	return nil
}

func (s *localStorageStore) DeleteSample(id string) error {
	if id == "" {
		return nil
	}
	sampleCacheMu.Lock()
	delete(sampleCache, id)
	sampleCacheMu.Unlock()
	if fn := js.Global().Get("idbDeleteSample"); fn.Truthy() {
		fn.Invoke(id)
	}
	return nil
}

// hydrateSamplesOnce loads all persisted samples from IndexedDB into the cache,
// blocking on the read Promise. Idempotent.
func hydrateSamplesOnce() {
	sampleCacheMu.Lock()
	if sampleHydrated {
		sampleCacheMu.Unlock()
		return
	}
	sampleHydrated = true
	sampleCacheMu.Unlock()

	fn := js.Global().Get("idbGetAllSamples")
	if !fn.Truthy() {
		return
	}
	done := make(chan struct{})
	then := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if len(args) > 0 && args[0].Truthy() {
			arr := args[0]
			n := arr.Get("length").Int()
			sampleCacheMu.Lock()
			for i := 0; i < n; i++ {
				e := arr.Index(i)
				id := e.Get("id").String()
				u8 := e.Get("bytes")
				if id == "" || !u8.Truthy() {
					continue
				}
				blen := u8.Get("length").Int()
				buf := make([]byte, blen)
				if blen > 0 {
					js.CopyBytesToGo(buf, u8)
				}
				sampleCache[id] = SampleBlob{SampleRate: e.Get("sr").Int(), PCM: buf}
			}
			sampleCacheMu.Unlock()
		}
		close(done)
		return nil
	})
	catch := js.FuncOf(func(_ js.Value, _ []js.Value) interface{} { close(done); return nil })
	defer then.Release()
	defer catch.Release()
	fn.Invoke().Call("then", then, catch)
	<-done
}
