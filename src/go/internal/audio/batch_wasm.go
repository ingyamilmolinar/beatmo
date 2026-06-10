//go:build js && wasm && !test

package audio

import (
	"sync"
	"syscall/js"
)

// batchScratch reuses the six []any slices across PlayBatch calls so
// the WASM Go→JS bridge does not allocate fresh argument slices per
// batch. Each js.ValueOf(slice) creates a JS-side Array + a Go-side
// js.Value with a finalizer; pre-fix, we leaked these at the audio
// event rate (60+ /sec) which contributed to long-session OOMs by
// growing WASM linear memory faster than Go GC could reclaim. Owned
// behind a Mutex because PlayBatch may be called from the audioLoop
// goroutine OR (in tests) directly from the UI goroutine; contention
// is essentially zero in practice.
type batchScratch struct {
	mu      sync.Mutex
	ids     []any
	vols    []any
	pitches []any
	durs    []any
	whens   []any
	hasWhen []any
}

var batchScratchPool batchScratch

// ensureLen resizes the six scratch slices to exactly n elements,
// growing the backing arrays only when n exceeds capacity. Reset cells
// to nil/zero so a smaller batch doesn't carry over stale data.
func (s *batchScratch) ensureLen(n int) {
	if cap(s.ids) < n {
		s.ids = make([]any, n)
		s.vols = make([]any, n)
		s.pitches = make([]any, n)
		s.durs = make([]any, n)
		s.whens = make([]any, n)
		s.hasWhen = make([]any, n)
		return
	}
	s.ids = s.ids[:n]
	s.vols = s.vols[:n]
	s.pitches = s.pitches[:n]
	s.durs = s.durs[:n]
	s.whens = s.whens[:n]
	s.hasWhen = s.hasWhen[:n]
}

// PlayBatch dispatches multiple sound requests via a single JS bridge call
// when available to reduce Go→JS crossings. Falls back to per-item dispatch.
//
// Prefers the playSoundsBatchFlat JS export (one Invoke with primitive
// arrays) over playSoundsBatch (one Invoke with Object-per-item — much
// heavier JS allocation). The historical BEATMO_AUDIO_BATCH_FLAT gate
// is gone: the flat path is unconditionally preferred when available.
func PlayBatch(reqs []BatchParam) {
	n := len(reqs)
	if n == 0 {
		return
	}
	// Stamp the trigger clock for every scheduled hit before dispatching to the
	// JS bridge, so sequencer-driven playback advances audio.SinceLastTrigger on
	// WASM exactly like the desktop batch path (which records via PlayParamsAt).
	// Without this, trigger-driven UI (synth glow, sampler playhead) only lit up
	// for the Preview button in the browser, never for the running sequencer.
	for i := range reqs {
		RecordVoiceTrigger(reqs[i].ID)
	}
	fnFlat := js.Global().Get("playSoundsBatchFlat")
	if fnFlat.Truthy() {
		batchScratchPool.mu.Lock()
		batchScratchPool.ensureLen(n)
		s := &batchScratchPool
		for i := range reqs {
			r := reqs[i]
			s.ids[i] = r.ID
			s.vols[i] = r.Vol
			s.pitches[i] = r.Pitch
			s.durs[i] = r.Dur
			if r.HasWhen {
				s.hasWhen[i] = true
				s.whens[i] = r.When
			} else {
				s.hasWhen[i] = nil
				s.whens[i] = nil
			}
		}
		jsIDs := js.ValueOf(s.ids)
		jsVols := js.ValueOf(s.vols)
		jsPitches := js.ValueOf(s.pitches)
		jsDurs := js.ValueOf(s.durs)
		jsWhens := js.ValueOf(s.whens)
		jsHasWhen := js.ValueOf(s.hasWhen)
		batchScratchPool.mu.Unlock()
		fnFlat.Invoke(jsIDs, jsVols, jsPitches, jsDurs, jsWhens, jsHasWhen)
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
