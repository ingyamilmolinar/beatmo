//go:build js && wasm && !test

package audio

import (
	"sync"
	"syscall/js"
)

// Reused argument slices: each js.ValueOf(slice) makes a JS Array plus a finalized js.Value, which at the event
// rate grew WASM memory faster than GC reclaimed it. Locked because tests call PlayBatch from the UI goroutine.
type batchScratch struct {
	mu      sync.Mutex
	ids     []any
	vols    []any
	pitches []any
	durs    []any
	whens   []any
	hasWhen []any
	rows    []any
	abss    []any
	ideals  []any
	grids   []any
	vids    []any
}

var batchScratchPool batchScratch

// Cells are reset so a smaller batch carries no stale data.
func (s *batchScratch) ensureLen(n int) {
	if cap(s.ids) < n {
		s.ids = make([]any, n)
		s.vols = make([]any, n)
		s.pitches = make([]any, n)
		s.durs = make([]any, n)
		s.whens = make([]any, n)
		s.hasWhen = make([]any, n)
		s.rows = make([]any, n)
		s.abss = make([]any, n)
		s.ideals = make([]any, n)
		s.grids = make([]any, n)
		s.vids = make([]any, n)
		return
	}
	s.ids = s.ids[:n]
	s.vols = s.vols[:n]
	s.pitches = s.pitches[:n]
	s.durs = s.durs[:n]
	s.whens = s.whens[:n]
	s.hasWhen = s.hasWhen[:n]
	s.rows = s.rows[:n]
	s.abss = s.abss[:n]
	s.ideals = s.ideals[:n]
	s.grids = s.grids[:n]
	s.vids = s.vids[:n]
}

// playSoundsBatchFlat (one Invoke with primitive arrays) over playSoundsBatch (an object per item, far heavier in
// JS).
func PlayBatch(reqs []BatchParam) {
	n := len(reqs)
	if n == 0 {
		return
	}
	// Stamped before dispatch, as desktop's PlayParamsAt does, or trigger-driven UI lights only for Preview in the
	// browser.
	for i := range reqs {
		// Lane voices are committed up to the horizon early; the UI lane stamps their trigger when the beat comes due.
		if reqs[i].VoiceID == 0 {
			RecordVoiceTrigger(reqs[i].ID)
		}
	}
	fnFlat := js.Global().Get("playSoundsBatchFlat")
	if fnFlat.Truthy() {
		batchScratchPool.mu.Lock()
		batchScratchPool.ensureLen(n)
		s := &batchScratchPool
		for i := range reqs {
			r := reqs[i]
			s.ids[i] = string(r.ID) // syscall/js panics on a named string type
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
			// -1 for fire-and-forget, sent explicitly rather than relying on sparse-array holes across the bridge.
			s.rows[i] = r.Row
			s.abss[i] = r.Abs
			// Measurement only, for the playback probe; 0 = unknown.
			s.ideals[i] = r.Ideal
			s.grids[i] = r.Grid
			// 0 = preview / UI sound, never cancelled.
			s.vids[i] = float64(r.VoiceID)
		}
		jsIDs := js.ValueOf(s.ids)
		jsVols := js.ValueOf(s.vols)
		jsPitches := js.ValueOf(s.pitches)
		jsDurs := js.ValueOf(s.durs)
		jsWhens := js.ValueOf(s.whens)
		jsHasWhen := js.ValueOf(s.hasWhen)
		jsRows := js.ValueOf(s.rows)
		jsAbss := js.ValueOf(s.abss)
		jsIdeals := js.ValueOf(s.ideals)
		jsGrids := js.ValueOf(s.grids)
		jsVids := js.ValueOf(s.vids)
		batchScratchPool.mu.Unlock()
		fnFlat.Invoke(jsIDs, jsVols, jsPitches, jsDurs, jsWhens, jsHasWhen, jsRows, jsAbss, jsIdeals, jsGrids, jsVids)
		return
	}
	fn := js.Global().Get("playSoundsBatch")
	if fn.Truthy() {
		arr := js.Global().Get("Array").New()
		for i := range reqs {
			r := reqs[i]
			o := js.Global().Get("Object").New()
			o.Set("id", string(r.ID))
			o.Set("vol", r.Vol)
			o.Set("pitch", r.Pitch)
			o.Set("dur", r.Dur)
			o.Set("row", r.Row)
			o.Set("abs", r.Abs)
			o.Set("ideal", r.Ideal)
			o.Set("grid", r.Grid)
			o.Set("vid", float64(r.VoiceID))
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
