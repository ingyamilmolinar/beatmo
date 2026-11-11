//go:build js && !test

package ui

import "syscall/js"

// platformVibrate calls the browser's `navigator.vibrate` API. Silently
// no-ops if the browser doesn't support vibration (desktop browsers,
// locked-down WebViews) — there is no reasonable fallback for haptics on
// those platforms.
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
