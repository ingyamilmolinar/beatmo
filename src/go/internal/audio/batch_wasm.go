//go:build js && wasm && !test

package audio

import "syscall/js"

// PlayBatch dispatches multiple sound requests via a single JS bridge call
// when available to reduce Go→JS crossings. Falls back to per-item dispatch.
func PlayBatch(reqs []BatchParam) {
	fn := js.Global().Get("playSoundsBatch")
	if fn.Truthy() {
		arr := js.Global().Get("Array").New()
		for i := range reqs {
			r := reqs[i]
			o := js.Global().Get("Object").New()
			o.Set("id", r.ID)
			o.Set("vol", r.Vol)
			o.Set("pitch", r.Pitch)
			o.Set("dur", r.Dur)
			if r.HasWhen {
				o.Set("when", r.When)
			}
			arr.Call("push", o)
		}
		fn.Invoke(arr)
		return
	}
	// Fallback path if batch API is unavailable.
	for i := range reqs {
		r := reqs[i]
		when := 0.0
		if r.HasWhen {
			when = r.When
		}
		PlayParamsAt(r.ID, r.Vol, r.Pitch, r.Dur, when)
	}
}
