//go:build js

package audio

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// PrewarmVoice renders (id, pitch) via the JS render worker. It is fire-and-
// forget; the gated warm-up uses WarmVoiceItem instead, which reports
// completion. Before 2026-09-13 this was a no-op, so the browser did no
// Go-driven warming at all.
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

// PrewarmVoiceDoc mirrors PrewarmVoice on browser: the document's block is
// pushed to audio.js separately (setInstrumentBlock), so it is already live by
// the time the render runs.
func PrewarmVoiceDoc(id instrument.ID, pitch float64, d instrument.Doc) {
	PrewarmVoice(id, pitch)
}

// VoiceCached queries the JS renderCache through the SAME key the trigger path
// reads. Returning a hardcoded false (the pre-2026-09-13 behaviour) made every
// already-warm voice look cold, so the warm-up could not skip anything.
func VoiceCached(id instrument.ID, pitch float64) bool {
	fn := js.Global().Get("voiceCachedFor")
	if !fn.Truthy() {
		return false
	}
	return fn.Invoke(string(id), pitch).Truthy()
}

// VoiceReady reports whether a trigger of (id, pitch) would find its voice
// rendered (spec 2026-10-31 §1.3; audio.js voiceReadyFor).
func VoiceReady(id instrument.ID, pitch float64) bool {
	fn := js.Global().Get("voiceReadyFor")
	if !fn.Truthy() {
		return true
	}
	return fn.Invoke(string(id), pitch).Truthy()
}

// WarmsPerPitch reports whether id renders at-pitch, so only melodic
// instruments get per-pitch warm items. Delegates to audio.js, which owns the
// RENDER_INFO table the render path uses.
func WarmsPerPitch(id instrument.ID) bool {
	fn := js.Global().Get("isMelodicInstrumentJS")
	if !fn.Truthy() {
		return false
	}
	return fn.Invoke(string(id)).Truthy()
}

// ─── Gated warm-up completion plumbing ─────────────────────────────────────
//
// NOTE on design: an earlier draft registered a fresh js.FuncOf per warm item
// (js.Global().Set("__beatmoWarmBatchDone", js.FuncOf(...))). That leaks a
// Go-side handle PER CALL — js.FuncOf handles are only freed by an explicit
// Release(), so a 91-item import would leak 91 handles — and since
// WarmVoiceItem is called once per item, each call would also overwrite the
// previous global callback (an overwrite race that only "worked" because the
// closures happened to be shape-identical). Instead we register ONE
// persistent callback at first use and route completions through a
// mutex-guarded token map, matching this codebase's one-registration
// convention (see filepicker_wasm.go, js_bootstrap_wasm.go).
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
			// A token that arrives twice (shouldn't happen, but JS is not
			// trusted to be perfectly disciplined) or an unknown token is a
			// safe no-op — never a panic, never a double-count against the
			// warm-up controller's completion tally.
			if ok && done != nil {
				done()
			}
			return nil
		}))
	})
}

// WarmVoiceItem renders (id, pitch) through warmInstrumentBatch and calls done
// exactly once, on EVERY path: missing JS function, JSON-encode failure,
// render success, and render failure. warmupController.itemDone credits the
// gate's completion counter only when done() fires — a dropped done() leaves
// the readiness gate waiting out its full deadline (warmupDeadlineMS) instead
// of opening, which is a user-visible hang, not a cosmetic bug.
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

// SetDispatchMustNotRender is a no-op on browser (renders already run on the
// render worker; there is no dispatch-goroutine render path to gate).
func SetDispatchMustNotRender(on bool) {}

// SetAsyncRenderSubmit is a no-op on browser, mirroring SetDispatchMustNotRender.
func SetAsyncRenderSubmit(f func(id instrument.ID, pitch float64, d instrument.Doc)) {}
