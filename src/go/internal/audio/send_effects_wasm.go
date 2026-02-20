//go:build js && wasm && !test

package audio

import "syscall/js"

// SetDelaySend sets the delay send amount (0-1) for an instrument channel (WASM).
func SetDelaySend(id string, amount float64) {
	if amount < 0 {
		amount = 0
	}
	if amount > 1 {
		amount = 1
	}
	fn := js.Global().Get("setDelaySend")
	if fn.Truthy() {
		fn.Invoke(id, amount)
	}
}

// DelaySend returns the delay send amount for an instrument (WASM).
func DelaySend(id string) float64 {
	fn := js.Global().Get("delaySend")
	if fn.Truthy() {
		return fn.Invoke(id).Float()
	}
	return 0
}

// SetReverbSend sets the reverb send amount (0-1) for an instrument channel (WASM).
func SetReverbSend(id string, amount float64) {
	if amount < 0 {
		amount = 0
	}
	if amount > 1 {
		amount = 1
	}
	fn := js.Global().Get("setReverbSend")
	if fn.Truthy() {
		fn.Invoke(id, amount)
	}
}

// ReverbSend returns the reverb send amount for an instrument (WASM).
func ReverbSend(id string) float64 {
	fn := js.Global().Get("reverbSend")
	if fn.Truthy() {
		return fn.Invoke(id).Float()
	}
	return 0
}
