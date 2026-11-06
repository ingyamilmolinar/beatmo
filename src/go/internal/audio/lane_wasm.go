//go:build js && wasm && !test

package audio

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// CancelVoices silently removes not-yet-started lane voices (audio.js
// cancelLaneVoices: started ones are left alone; queued / deferred ones are
// tombstoned).
// CancelVoices removes not-yet-started lane voices in audio.js and returns
// the ones it refused because they had already started.
func CancelVoices(ids []uint64) (started []uint64) {
	if len(ids) == 0 {
		return nil
	}
	fn := js.Global().Get("cancelLaneVoices")
	if !fn.Truthy() {
		return nil
	}
	arr := make([]any, len(ids))
	for i, v := range ids {
		arr[i] = float64(v) // vids stay far below 2^53
	}
	res := fn.Invoke(js.ValueOf(arr))
	if res.Type() != js.TypeObject {
		return nil
	}
	for i, n := 0, res.Length(); i < n; i++ {
		started = append(started, uint64(res.Index(i).Float()))
	}
	return started
}

// StopLaneAt fades, at ctx time t, the lane voices of inst that started
// before t (a mute node), sparing voices that start at/after t.
func StopLaneAt(inst instrument.ID, t float64) {
	if fn := js.Global().Get("stopLaneAt"); fn.Truthy() {
		fn.Invoke(string(inst), t)
	}
}

// CancelAllLaneFuture removes every lane voice that has not started yet.
func CancelAllLaneFuture() {
	if fn := js.Global().Get("cancelAllLaneFuture"); fn.Truthy() {
		fn.Invoke()
	}
}
