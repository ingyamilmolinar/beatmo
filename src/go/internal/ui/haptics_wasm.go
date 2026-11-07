//go:build js && !test

package ui

import "syscall/js"

// Silently a no-op where vibration is unsupported; there is no reasonable fallback.
func platformVibrate(ms int) {
	if ms <= 0 {
		return
	}
	nav := js.Global().Get("navigator")
	if nav.IsUndefined() || nav.IsNull() {
		return
	}
	vib := nav.Get("vibrate")
	if vib.Type() != js.TypeFunction {
		return
	}
	nav.Call("vibrate", ms)
}
