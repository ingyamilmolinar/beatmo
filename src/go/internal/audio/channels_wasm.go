//go:build js && wasm

package audio

import "syscall/js"

func init() {
	platformChannelVolumeChanged = func(id string, vol float64) {
		fn := js.Global().Get("setChannelVolume")
		if fn.Truthy() {
			fn.Invoke(id, vol)
		}
	}
}
