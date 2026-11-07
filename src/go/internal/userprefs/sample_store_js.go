//go:build js

package userprefs

import (
	"sync"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// PCM payloads exceed localStorage's ~5 MB, so samples live in IndexedDB (audio.js shims). IndexedDB is async and the
// SampleStore surface is not, so a cache is hydrated once (blocking on the Promise) and written through; the bootstrap
// hydrates before the first Layout.
var (
	sampleCacheMu  sync.RWMutex
	sampleCache    = map[instrument.ID]SampleBlob{}
	sampleHydrated bool
)

func (s *localStorageStore) LoadSamples() (map[instrument.ID]SampleBlob, error) {
	hydrateSamplesOnce()
	sampleCacheMu.RLock()
	defer sampleCacheMu.RUnlock()
	out := make(map[instrument.ID]SampleBlob, len(sampleCache))
	for id, b := range sampleCache {
		out[id] = SampleBlob{SampleRate: b.SampleRate, PCM: append([]byte(nil), b.PCM...)}
	}
	return out, nil
}

func (s *localStorageStore) SaveSample(id instrument.ID, blob SampleBlob) error {
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
		fn.Invoke(string(id), blob.SampleRate, u8)
	}
	return nil
}

func (s *localStorageStore) DeleteSample(id instrument.ID) error {
	if id == "" {
		return nil
	}
	sampleCacheMu.Lock()
	delete(sampleCache, id)
	sampleCacheMu.Unlock()
	if fn := js.Global().Get("idbDeleteSample"); fn.Truthy() {
		fn.Invoke(string(id))
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
				id, err := instrument.ParseID(e.Get("id").String())
				u8 := e.Get("bytes")
				if err != nil || !u8.Truthy() {
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
