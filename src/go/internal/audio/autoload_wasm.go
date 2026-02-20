//go:build js && wasm && !test

package audio

import (
	"sync"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
)

// AutoLoadEmbeddedWAVs registers all embedded WAVs by creating Blob URLs and
// forwarding them to the JS bridge's loadWav(id, url). We also append the IDs
// to the instrument list so the UI can display them.
var (
	samplesTotal      int
	samplesRegistered int
	samplesMu         sync.RWMutex
)

// SampleLoadProgress returns (loaded,total) for embedded sample registration.
// Total is the number of embedded WAVs; loaded increases as URL registrations
// are completed. Decoding is deferred to first use.
func SampleLoadProgress() (int, int) {
	samplesMu.RLock()
	defer samplesMu.RUnlock()
	return samplesRegistered, samplesTotal
}

func AutoLoadEmbeddedWAVs() {
	wavs, err := assets.ListEmbeddedWAVs()
	if err != nil {
		return
	}
	samplesMu.Lock()
	samplesTotal = len(wavs)
	samplesRegistered = 0
	samplesMu.Unlock()
	// Phase 1: Immediately register IDs so the UI can list instruments.
	for _, w := range wavs {
		Register(w.ID, nil)
	}
	// Phase 2: Lazily create Blob URLs and register them on the JS side so
	// decoding happens on first use. Spread work across macrotasks to avoid
	// blocking startup.
	g := js.Global()
	uint8Array := g.Get("Uint8Array")
	blobCtor := g.Get("Blob")
	url := g.Get("URL")
	setTimeout := g.Get("setTimeout")
	delay := 0
	for _, w := range wavs {
		// capture for closure
		id := w.ID
		data := make([]byte, len(w.Data))
		copy(data, w.Data)
		// Build a retrying attempt that registers the URL when JS is ready.
		var attempt func()
		attempt = func() {
			g := js.Global()
			reg := g.Get("registerWav")
			load := g.Get("loadWav")
			if reg.Truthy() || load.Truthy() {
				buf := uint8Array.New(len(data))
				js.CopyBytesToJS(buf, data)
				blob := blobCtor.New([]interface{}{buf})
				u := url.Call("createObjectURL", blob)
				if reg.Truthy() {
					reg.Invoke(id, u)
					samplesMu.Lock()
					samplesRegistered++
					samplesMu.Unlock()
				} else {
					// Fallback: decode now via loadWav(id, url) and advance when finished.
					p := load.Invoke(id, u)
					if p.Truthy() {
						var onDone js.Func
						onDone = js.FuncOf(func(this js.Value, args []js.Value) any {
							onDone.Release()
							samplesMu.Lock()
							samplesRegistered++
							samplesMu.Unlock()
							return nil
						})
						// then(success, failure)
						p.Call("then", onDone, onDone)
					} else {
						// Non-promise fallback
						samplesMu.Lock()
						samplesRegistered++
						samplesMu.Unlock()
					}
				}
				return
			}
			if setTimeout.Truthy() {
				var f js.Func
				f = js.FuncOf(func(this js.Value, args []js.Value) any { defer f.Release(); attempt(); return nil })
				setTimeout.Invoke(f, 50)
			}
		}
		if setTimeout.Truthy() {
			var f js.Func
			f = js.FuncOf(func(this js.Value, args []js.Value) any { defer f.Release(); attempt(); return nil })
			setTimeout.Invoke(f, delay)
		} else {
			attempt()
		}
		if delay < 500 {
			delay += 25 // stagger registrations
		}
	}
}
