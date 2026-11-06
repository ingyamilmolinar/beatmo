//go:build js && wasm

package audio

import (
	"encoding/json"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

func init() {
	platformChannelVolumeChanged = func(id channel.ID, vol float64) {
		if id.IsZero() {
			return // names no channel: never "" to JS, which reads it as the master bus
		}
		fn := js.Global().Get("setChannelVolume")
		if fn.Truthy() {
			fn.Invoke(id.String(), vol)
		}
	}
	platformRenderBudget = func(n int64) {
		if fn := js.Global().Get("setRenderCacheBudget"); fn.Truthy() {
			fn.Invoke(float64(n))
		}
	}
	platformHeldChanged = func(ids []instrument.ID) {
		b, err := json.Marshal(ids)
		if err != nil {
			return
		}
		if fn := js.Global().Get("setHeldChannels"); fn.Truthy() {
			fn.Invoke(string(b))
		}
	}
}
