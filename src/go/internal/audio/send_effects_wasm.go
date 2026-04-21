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

// ConfigureSendDelay reconfigures the global send delay (WASM).
// Delegates to JS configureSendDelay(timeMs, feedback, dampingHz) if available.
func ConfigureSendDelay(timeMs, feedback, dampingHz float64) {
	fn := js.Global().Get("configureSendDelay")
	if fn.Truthy() {
		fn.Invoke(timeMs, feedback, dampingHz)
	}
}

// ConfigureSendReverb reconfigures the global send reverb (WASM).
// Delegates to JS configureSendReverb(room, damping, wet) if available.
func ConfigureSendReverb(room, damping, wet float64) {
	fn := js.Global().Get("configureSendReverb")
	if fn.Truthy() {
		fn.Invoke(room, damping, wet)
	}
}

// SendDelayParams returns the current delay send configuration (WASM).
// Delegates to JS sendDelayParams() if available; returns defaults otherwise.
func SendDelayParams() (float64, float64, float64) {
	fn := js.Global().Get("sendDelayParams")
	if fn.Truthy() {
		res := fn.Invoke()
		return res.Index(0).Float(), res.Index(1).Float(), res.Index(2).Float()
	}
	return 300, 0.3, 3000
}

// SendReverbParams returns the current reverb send configuration (WASM).
// Delegates to JS sendReverbParams() if available; returns defaults otherwise.
func SendReverbParams() (float64, float64, float64) {
	fn := js.Global().Get("sendReverbParams")
	if fn.Truthy() {
		res := fn.Invoke()
		return res.Index(0).Float(), res.Index(1).Float(), res.Index(2).Float()
	}
	return 0.7, 0.4, 0.3
}
