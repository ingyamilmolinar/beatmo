//go:build js && wasm && !test

package audio

import "syscall/js"

// PlayBatch dispatches multiple sound requests via a single JS bridge call
// when available to reduce Go→JS crossings. Falls back to per-item dispatch.
func PlayBatch(reqs []BatchParam) {
	fnFlat := js.Global().Get("playSoundsBatchFlat")
	if fnFlat.Truthy() && js.Global().Get("BEATMO_AUDIO_BATCH_FLAT").Truthy() {
		n := len(reqs)
		if n == 0 {
			return
		}
		ids := make([]any, n)
		vols := make([]any, n)
		pitches := make([]any, n)
		durs := make([]any, n)
		whens := make([]any, n)
		hasWhen := make([]any, n)
		for i := range reqs {
			r := reqs[i]
			ids[i] = r.ID
			vols[i] = r.Vol
			pitches[i] = r.Pitch
			durs[i] = r.Dur
			if r.HasWhen {
				hasWhen[i] = true
				whens[i] = r.When
			}
		}
		jsVols := js.ValueOf(vols)
		jsPitches := js.ValueOf(pitches)
		jsDurs := js.ValueOf(durs)
		jsWhens := js.ValueOf(whens)
		jsHasWhen := js.ValueOf(hasWhen)
		fnFlat.Invoke(js.ValueOf(ids), jsVols, jsPitches, jsDurs, jsWhens, jsHasWhen)
		return
	}
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
