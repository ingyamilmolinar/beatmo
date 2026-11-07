//go:build js

package audio

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Fire-and-forget; the gated warm-up uses WarmVoiceItem, which reports completion.
func PrewarmVoice(id instrument.ID, pitch float64) {
	if id == "" {
		return
	}
	fn := js.Global().Get("warmInstrument")
	if !fn.Truthy() {
		return
	}
	data, err := json.Marshal([]float64{pitch})
	if err != nil {
		return
	}
	fn.Invoke(string(id), string(data))
}

// The document's block is already pushed to audio.js by the time the render runs.
func PrewarmVoiceDoc(id instrument.ID, pitch float64, d instrument.Doc) {
	PrewarmVoice(id, pitch)
}

// Through the SAME key the trigger reads; a constant false would make every warm voice look cold and the warm-up skip
// nothing.
func VoiceCached(id instrument.ID, pitch float64) bool {
	fn := js.Global().Get("voiceCachedFor")
	if !fn.Truthy() {
		return false
	}
	return fn.Invoke(string(id), pitch).Truthy()
}

// audio.js voiceReadyFor.
func VoiceReady(id instrument.ID, pitch float64) bool {
	fn := js.Global().Get("voiceReadyFor")
	if !fn.Truthy() {
		return true
	}
	return fn.Invoke(string(id), pitch).Truthy()
}

// Only melodic instruments get per-pitch warm items; audio.js owns the table the render path uses.
func WarmsPerPitch(id instrument.ID) bool {
	fn := js.Global().Get("isMelodicInstrumentJS")
	if !fn.Truthy() {
		return false
	}
	return fn.Invoke(string(id)).Truthy()
}

// ONE persistent callback routing completions through a token map: a js.FuncOf per item leaks a handle each (freed only
// by Release) and races the previous global.
var (
	warmDoneOnce    sync.Once
	warmDoneMu      sync.Mutex
	warmDonePending = map[int64]func(){}
	warmTokenSeq    int64
)

// ensureWarmDoneRegistered installs the __beatmoWarmDone global exactly once.
func ensureWarmDoneRegistered() {
	warmDoneOnce.Do(func() {
		js.Global().Set("__beatmoWarmDone", js.FuncOf(func(this js.Value, args []js.Value) any {
			if len(args) == 0 {
				return nil
			}
			token := int64(args[0].Int())
			warmDoneMu.Lock()
			done, ok := warmDonePending[token]
			if ok {
				delete(warmDonePending, token)
			}
			warmDoneMu.Unlock()
			// A duplicate or unknown token is a safe no-op: never a panic, never a double count.
			if ok && done != nil {
				done()
			}
			return nil
		}))
	})
}

// done fires exactly once on EVERY path: a dropped done() leaves the readiness gate waiting out its whole deadline, a
// user-visible hang.
func WarmVoiceItem(id instrument.ID, pitch float64, done func()) {
	if done == nil {
		return
	}
	if id == "" {
		done()
		return
	}
	fn := js.Global().Get("warmInstrumentBatch")
	if !fn.Truthy() {
		done()
		return
	}
	ensureWarmDoneRegistered()
	token := atomic.AddInt64(&warmTokenSeq, 1)
	warmDoneMu.Lock()
	warmDonePending[token] = done
	warmDoneMu.Unlock()
	data, err := json.Marshal([][3]any{{id, pitch, token}})
	if err != nil {
		warmDoneMu.Lock()
		delete(warmDonePending, token)
		warmDoneMu.Unlock()
		done()
		return
	}
	fn.Invoke(string(data), "__beatmoWarmDone")
}

// Browser renders already run on the render worker: there is no dispatch render to gate.
func SetDispatchMustNotRender(on bool) {}

// SetAsyncRenderSubmit is a no-op on browser, mirroring SetDispatchMustNotRender.
func SetAsyncRenderSubmit(f func(id instrument.ID, pitch float64, d instrument.Doc)) {}
