//go:build js && wasm

package audio

import (
	"encoding/binary"
	"math"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// audio.js renders the pushed block as-is, patching only the pitch index for melodic per-pitch renders.

func init() {
	platformInstrumentDocChanged = pushInstrumentDoc
	platformInstrumentDocRemoved = func(id instrument.ID) {
		if fn := js.Global().Get("clearInstrumentBlock"); fn.Type() == js.TypeFunction {
			fn.Invoke(string(id))
		}
	}
}

func pushInstrumentDoc(id instrument.ID, d instrument.Doc) {
	fn := js.Global().Get("setInstrumentBlock")
	if fn.Type() != js.TypeFunction {
		return // audio.js not loaded (Go-only harness)
	}
	block := instrument.Flatten(d, BuiltinModuleRegistry())
	raw := make([]byte, 8*len(block))
	for i, x := range block {
		binary.LittleEndian.PutUint64(raw[8*i:], math.Float64bits(x))
	}
	f64 := js.Global().Get("Float64Array").New(len(block))
	js.CopyBytesToJS(js.Global().Get("Uint8Array").New(f64.Get("buffer")), raw)
	fn.Invoke(string(id), f64, d.Render.Seconds, d.Render.Amplitude, d.Render.Melodic, d.Render.GateTail, voiceFXForJS(d))
}

// Each applied by the render worker through the C host's ifx_create.
func voiceFXForJS(d instrument.Doc) js.Value {
	calls := VoiceFXCalls(d)
	out := make([]any, len(calls))
	for i, c := range calls {
		params := make(map[string]any, len(c.Params))
		for k, v := range c.Params {
			params[k] = v
		}
		out[i] = map[string]any{"type": c.Type, "params": params}
	}
	return js.ValueOf(out)
}

// At boot, and after a sample override is dropped so the synth voice comes back.
func PushAllInstrumentDocsToPlatform() {
	docStore.RLock()
	snap := make(map[instrument.ID]instrument.Doc, len(docStore.docs))
	for k, v := range docStore.docs {
		snap[k] = v.doc
	}
	docStore.RUnlock()
	for id, d := range snap {
		pushInstrumentDoc(id, d)
	}
}

// pushInstrumentDocByID re-pushes one instrument's current document.
func pushInstrumentDocByID(id instrument.ID) {
	if d, ok := InstrumentDoc(id); ok {
		pushInstrumentDoc(id, d)
	}
}
